package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sq325/cmdDaemon/config"
	"github.com/sq325/cmdDaemon/daemon"
	"github.com/sq325/cmdDaemon/register"
	"github.com/sq325/cmdDaemon/web/handler"

	_ "github.com/sq325/cmdDaemon/docs"

	"context"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	fork "github.com/sevlyar/go-daemon"
	"github.com/spf13/pflag"

	swaggerFiles "github.com/swaggo/files"

	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	complementConsul "github.com/sq325/kitComplement/consul"
	tool "github.com/sq325/kitComplement/tool"
	ginSwagger "github.com/swaggo/gin-swagger"

	daemontool "github.com/sq325/cmdDaemon/internal/tool"
)

const (
	_service = "cmddaemon"
)

var (
	_versionInfo   string
	buildTime      string
	buildGoVersion string
	_version       string
	author         string
)

// flags
var (
	createConfFile *bool     = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile     *string   = pflag.String("config.file", "./daemon.yml", "Daemon configuration file name.")
	version        *bool     = pflag.BoolP("version", "v", false, "Print version information.")
	port           *string   = pflag.String("web.port", "9090", "Port to listen.")
	consulAddr     *string   = pflag.String("consul.addr", "", "Consul address. e.g. localhost:8500")
	registerCmds   *bool     = pflag.Bool("consul.regChild", false, "Register all child processes to consul.")
	svcIP          *string   = pflag.String("svcIP", "", "svc ip, default hostAdmIp")
	consulIfList   *[]string = pflag.StringSlice("consul.infList", []string{"bond0", "eth0", "eth1"}, `Network interface list. e.g. --consul.infList="v1,v2"`)
	// consulSvcRegFile *string   = pflag.String("consul.svcRegFile", "./services.json", "Consul service register file name.")
	logLevel *string = pflag.String("log.level", "info", "Log level. e.g. debug, info, warn, error, dpanic, panic, fatal")

	printCmds *bool = pflag.BoolP("printCmds", "p", false, "Print cmds parse from config.")
	killCmds  *bool = pflag.Bool("killCmds", false, "Kill all child processes from config.")
	// printConsulConf *bool = pflag.Bool("printConsulConf", false, "Print consul config.")
)

var (
	conf *config.Conf

	signCh = make(chan os.Signal)
)

func init() {
	pflag.Parse()
}

// @title			守护进程服务
// @license.name	Apache 2.0
func main() {
	if *createConfFile {
		createConfigFile()
		return
	}
	if *version {
		fmt.Println(_service, _version)
		fmt.Println("build time:", buildTime)
		fmt.Println("go version:", buildGoVersion)
		fmt.Println("author:", author)
		fmt.Println("version info:", _versionInfo)
		return
	}

	// config init
	initConf()
	if *printCmds {
		cmds, _ := config.GenerateCmds(conf)
		if len(cmds) == 0 {
			fmt.Println("No cmd to print.")
			return
		}
		for _, cmd := range cmds {
			fmt.Println(cmd.String())
		}
		return
	}

	// Clear potential zombie processes based on the configuration file
	// only to be used when the daemon panics.
	if *killCmds {
		cmds, _ := config.GenerateCmds(conf)
		if len(cmds) == 0 {
			fmt.Println("No cmd to kill.")
			return
		}
		for _, cmd := range cmds {
			err := killcmd(cmd)
			if err != nil {
				fmt.Println(err)
			}
		}
		return
	}

	cntxt := newForkCtx()
	child, err := cntxt.Reborn()
	if err != nil {
		log.Fatal("Unable to run: ", err)
	}
	if child != nil { // 如果此时在父进程中，直接退出
		return
	}
	defer cntxt.Release() // 在函数结束时重启stdin、stdout、stderr

	fmt.Println("- - - - - - - - - - - - - - -")
	fmt.Printf("Daemon started %s\n", time.Now().Format(time.DateTime))

	// 初始化日志
	var logger *slog.Logger
	switch *logLevel {
	case "debug":
		logger = NewLogger(slog.LevelDebug)
	case "info":
		logger = NewLogger(slog.LevelInfo)
	case "warn":
		logger = NewLogger(slog.LevelWarn)
	case "error":
		logger = NewLogger(slog.LevelError)
	default:
		logger = NewLogger(slog.LevelInfo)
	}
	logger.Info("Daemon started.", "time", time.Now().Format(time.DateTime))
	logger.Info("Daemon config file", "file", *configFile)

	// config
	cmds, annotationsList := config.GenerateCmds(conf)
	if len(cmds) == 0 {
		logger.Error("No cmd to run. Daemon existed.")
		return
	}

	// signal
	signal.Notify(signCh, syscall.SIGHUP, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化Daemon

	dcmds := make([]*daemon.DaemonCmd, 0, len(cmds))
	for i, cmd := range cmds {
		dcmd := daemon.NewDaemonCmd(ctx, cmd, annotationsList[i])
		dcmds = append(dcmds, dcmd)
	}
	onceDaemon := sync.OnceValue(func() *daemon.Daemon {
		return createDaemon(ctx, dcmds, logger)
	})
	d := onceDaemon()
	logger.Info("Daemon created.")
	logger.Debug("daemon", "dcmds", fmt.Sprintf("%+v", d.DCmds))
	go d.Run()                  // run cmds
	time.Sleep(5 * time.Second) // wait for cmds running

	// 初始化svcManager
	svc := handler.NewSvcManager(logger, d)

	reg := prometheus.NewRegistry()
	{
		reg.Unregister(promcollectors.NewGoCollector())
		reg.MustRegister(httpRequestsTotal)
		reg.MustRegister(httpRequestDuration)
		reg.MustRegister(httpRequestErrorTotal)
	}
	d.RegisterMetrics(reg)
	metricsHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	healthSvc := func() bool {
		return svc.Health()
	}

	// 注册路由
	mux := gin.Default()
	gin.SetMode(gin.ReleaseMode)

	mux.Use(prometheusMiddleware())

	mux.PUT("/restart", func(c *gin.Context) {
		if err := svc.Restart(); err != nil {
			c.JSON(500, handler.SvcManagerResponse{Err: err.Error()})
			return
		}
		c.JSON(200, handler.SvcManagerResponse{V: "ok"})
	})

	mux.PUT("/reload", func(c *gin.Context) {
		err := svc.Reload()
		if err != nil {
			c.JSON(500, handler.SvcManagerResponse{Err: err.Error()})
			return
		}
		c.JSON(200, handler.SvcManagerResponse{V: "ok"})
	})

	mux.GET("/list", func(c *gin.Context) {
		data := svc.List()
		if data == nil {
			c.JSON(500, handler.SvcManagerResponse{Err: "No cmd to run."})
			return
		}
		c.Data(200, "application/json", data)
	})

	mux.PUT("/update", func(c *gin.Context) {
		if err := svc.Update(); err != nil {
			c.JSON(500, handler.SvcManagerResponse{Err: err.Error()})
			return
		}
		c.JSON(200, handler.SvcManagerResponse{V: "ok"})
	})

	mux.PUT("/stop", func(c *gin.Context) {
		if err := svc.Stop(); err != nil {
			c.JSON(500, handler.SvcManagerResponse{Err: err.Error()})
			return
		}
		c.JSON(200, handler.SvcManagerResponse{V: "ok"})
	})

	mux.Any("/health",
		func(c *gin.Context) {
			if healthSvc() {
				c.String(200, "%s service is healthy", _service)
				return
			}
			c.String(500, "%s service is unhealthy", _service)
		},
	)
	mux.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	mux.GET("/metrics", gin.WrapH(metricsHandler))

	// hostAdmIp
	var hostAdmIp string
	func() {
		var err error
		hostAdmIp, err = tool.HostAdmIp(*consulIfList)
		if err != nil {
			logger.Error("HostAdmIp err", "error", err, "intfList", *consulIfList)
		}
	}()

	// register service
	var r complementConsul.Registrar
	var node *register.Node
	if *consulAddr != "" {
		{
			c := strings.Split(strings.TrimSpace(*consulAddr), ":")
			ip := c[0]
			p, _ := strconv.Atoi(c[1])
			consulClient := complementConsul.NewConsulClient(ip, p)
			logger := complementConsul.NewLogger()
			r = complementConsul.NewRegistrar(consulClient, logger)
		}

		// register node
		if hostAdmIp != "" {
			node, err = register.NewNode(hostAdmIp)
		} else {
			node, err = register.NewNode(*svcIP)
		}
		if err != nil {
			logger.Error("NewNode err", "error", err)
		} else {
			err = node.Register(*consulAddr)
			if err != nil {
				logger.Error("Node Register err", "error", err)
			}
		}

		// register daemon svc
		var svc *complementConsul.Service
		{
			if *svcIP == "" {
				*svcIP, err = tool.HostAdmIp(*consulIfList)
				if err != nil {
					logger.Error("HostAdmIp err", "error", err, "intfList", *consulIfList)
				}
			}
			port, _ := strconv.Atoi(*port)
			svc = &complementConsul.Service{
				Name: _service,
				ID:   _service + "_" + node.Name,
				IP:   *svcIP,
				Port: port,
				Check: struct {
					Path     string
					Interval string
					Timeout  string
				}{
					Path:     "/health",
					Interval: "60s",
					Timeout:  "10s",
				},
			}
		}
		r.Register(svc)
		logger.Info("Daemon registered.")
		defer r.Deregister(svc)
	}

	// register cmds
	registerCmdsF := func() {
		if *consulAddr != "" && *registerCmds && len(d.DCmds) > 0 {
			pidAddrM, _ := daemontool.PidAddr()
			pattern := regexp.MustCompile(`prometheus_(?P<tag>[\w\.]+?)\.ya?ml`)
			for _, dcmd := range d.DCmds {
				if dcmd.Status == daemon.Exited {
					continue
				}

				path := dcmd.Cmd.Path
				cmdStr := dcmd.Cmd.String()
				pid := strconv.Itoa(dcmd.Cmd.Process.Pid)
				addr, ok := pidAddrM[pid]
				if !ok {
					logger.Error("pid not found", "pid", pid, "name", path)
					continue
				}
				port, err := strconv.Atoi(daemontool.Parseport(addr))
				if err != nil {
					logger.Error("Parseport err", "addr", addr, "error", err)
					continue
				}
				var (
					name      string
					tags      []string
					checkPath string
				)

				if strings.Contains(path, "prometheus") {
					tagIndex := pattern.SubexpIndex("tag")
					matches := pattern.FindStringSubmatch(cmdStr)
					if len(matches) > 0 {
						tags = append(tags, matches[tagIndex])
					}
					name = "prometheus" + ":" + strconv.Itoa(port)
					checkPath = "/-/healthy"
				} else if strings.Contains(path, "alertmanager") {
					name = "alertmanager"
					checkPath = "/-/healthy"
				} else {
					name = path
					checkPath = "/health"
				}
				cmdsvc := complementConsul.Service{
					Name: name,
					ID:   name + "_" + node.Name,
					IP:   *svcIP,
					Port: port,
					Tags: tags,
					Check: struct {
						Path     string
						Interval string
						Timeout  string
					}{
						Path:     checkPath,
						Interval: "60s",
						Timeout:  "10s",
					},
				}

				r.Register(&cmdsvc)
				logger.Info("Service registered", "name", cmdsvc.Name, "port", cmdsvc.Port)
			}
		}
	}
	deregisterCmdsF := func() {
		if *consulAddr != "" && *registerCmds && len(d.DCmds) > 0 {
			pidAddrM, _ := daemontool.PidAddr()
			pattern := regexp.MustCompile(`prometheus_(?P<tag>[\w\.]+?)\.ya?ml`)
			for _, dcmd := range d.DCmds {

				path := dcmd.Cmd.Path
				cmdStr := dcmd.Cmd.String()
				pid := strconv.Itoa(dcmd.Cmd.Process.Pid)
				addr, ok := pidAddrM[pid]
				if !ok {
					logger.Error("pid not found", "pid", pid, "name", path)
					continue
				}
				port, err := strconv.Atoi(daemontool.Parseport(addr))
				if err != nil {
					logger.Error("Parseport err", "addr", addr, "error", err)
					continue
				}
				var (
					name      string
					tags      []string
					checkPath string
				)

				if strings.Contains(path, "prometheus") {
					tagIndex := pattern.SubexpIndex("tag")
					matches := pattern.FindStringSubmatch(cmdStr)
					if len(matches) > 0 {
						tags = append(tags, matches[tagIndex])
					}
					name = "prometheus" + ":" + strconv.Itoa(port)
					checkPath = "/-/healthy"
				} else if strings.Contains(path, "alertmanager") {
					name = "alertmanager"
					checkPath = "/-/healthy"
				} else {
					name = path
					checkPath = "/health"
				}
				cmdsvc := complementConsul.Service{
					Name: name,
					ID:   name + "_" + node.Name,
					IP:   *svcIP,
					Port: port,
					Tags: tags,
					Check: struct {
						Path     string
						Interval string
						Timeout  string
					}{
						Path:     checkPath,
						Interval: "60s",
						Timeout:  "10s",
					},
				}

				r.Deregister(&cmdsvc)
				logger.Info("Service deregistered", "name", cmdsvc.Name, "port", cmdsvc.Port)
			}
		}
	}
	registerCmdsF()
	defer deregisterCmdsF()

	chDone := make(chan struct{}, 1)
	mux.Use(cors.Default())
	go func() { // no blocking
		err := mux.Run(":" + *port)
		if err != nil {
			logger.Error("mux.Run err", "error", err)
		}
		chDone <- struct{}{}
		close(chDone)
	}()

	// 防止子进程成为僵尸进程
	defer func() {
		pid := os.Getpid()
		cancel()
		syscall.Kill(-pid, syscall.SIGTERM)
		time.Sleep(5 * time.Second)
	}()

	// 捕捉信号
	for {
		select {
		case sig := <-signCh:
			switch sig {
			// 重新加载配置文件
			case syscall.SIGHUP:
				// recover initConf panic
				var initConfPanic bool
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("Reload config failed", "error", r)
							logger.Info("Panic Recover. Nothing changed.")
							initConfPanic = true
						}
					}()
					initConf()
				}()
				if initConfPanic { // if initConf panic, do not reload
					break
				}

				logger.Info("Reloaded config.")
				cmds, anntationsList := config.GenerateCmds(conf)
				if len(cmds) == 0 {
					logger.Error("No cmd to run. Do not reload.")
					break
				}
				// 关闭所有子进程
				cancel()
				var wg sync.WaitGroup
				for _, dcmd := range d.DCmds {
					dcmd := dcmd // capture range variable
					if dcmd.Status == daemon.Exited {
						continue
					}
					pid := dcmd.Cmd.Process.Pid
					err := syscall.Kill(pid, syscall.SIGTERM)
					if err != nil {
						logger.Error("Kill failed", "cmd", dcmd.Cmd.String(), "pid", pid, "error", err)
					}

					// wait for child process exited
					// if not exited, kill it after 10s
					wg.Add(1)
					go func() {
						defer wg.Done()
						// wait for child process exited
						if dcmd.Cmd == nil || dcmd.Cmd.ProcessState == nil {
							return
						}
						isExited := dcmd.Cmd.ProcessState.Exited()
						ch := make(chan struct{}, 1)
						if isExited {
							ch <- struct{}{}
						}
						select {
						case <-ch:
						case <-time.After(10 * time.Second):
							dcmd.Cmd.Process.Kill()
						}
					}()
				}
				wg.Wait()
				logger.Info("Ctx canceled. All child processes killed.")

				// reload Daemon and run new cmds
				ctx, cancel = context.WithCancel(context.Background())
				d.Reload(ctx, cmds, anntationsList)
				go d.Run()

				time.Sleep(10 * time.Second)
				registerCmdsF()
			// kill all child processes
			case syscall.SIGTERM:
				logger.Warn("Catched a term sign, kill all child processes", "time", time.Now().Format(time.DateTime))
				return // defer 会kill所有子进程
			}
		case <-chDone: // http server exited
			logger.Info("Web server exited.")
			return
		}
	}
}

// 守护进程上下文
func newForkCtx() *fork.Context {
	// commandName := os.Args[0]
	return &fork.Context{
		PidFileName: "daemon.pid",
		PidFilePerm: 0644,
		LogFileName: "daemon.log",
		LogFilePerm: 0644,
		WorkDir:     "./",
		Umask:       027,
		// Args:        []string{commandName},
	}
}

// 生成默认配置文件
func createConfigFile() {
	str := config.DefaultConfig
	_, err := os.Stat("daemon.yml")
	if err == nil || os.IsExist(err) {
		fmt.Println("daemon.yml existed")
		return
	}
	file, err := os.Create("daemon.yml")
	if err != nil {
		fmt.Println("create config.yml file err:", err)
		return
	}
	file.WriteString(str)
	fmt.Println("daemon.yml created.")
}

// 配置初始化
func initConf() {
	configBytes, err := os.ReadFile(*configFile)
	if err != nil {
		panic("Read config failed.")
	}
	conf, err = config.Unmarshal(configBytes)
	if err != nil {
		panic("Unmarshal config failed.")
	}
	if len(conf.Cmds) == 0 {
		panic("No cmd found.")
	}
}

// 初始化日志实例
// func NewLogger(level zapcore.Level) *zap.SugaredLogger {
// 	writer := os.Stdout
// 	encoderConfig := zap.NewProductionEncoderConfig()
// 	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 时间格式：2020-12-16T17:53:30.466+0800
// 	// encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder   // 时间格式：2020-12-16T17:53:30.466+0800
// 	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 在日志文件中使用大写字母记录日志级别
// 	encoder := zapcore.NewConsoleEncoder(encoderConfig)
// 	writeSyncer := zapcore.AddSync(writer)

// 	core := zapcore.NewCore(encoder, writeSyncer, level)

// 	logger := zap.New(core, zap.AddCaller()).Sugar() // AddCaller() 显示行号和文件名
// 	return logger
// }

func NewLogger(level slog.Level) *slog.Logger {
	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}))
	return l
}

func killcmd(cmd *exec.Cmd) error {
	psgrepStr := "ps -eo pid,command | grep " + "\"" + cmd.String() + "\"" + " | grep -v grep | awk '{print $1}'"
	psgrep := exec.Command("sh", "-c", psgrepStr)
	psgrepout, err := psgrep.Output()
	if err != nil {
		return err
	}
	if len(psgrepout) == 0 {
		return errors.New("psgrep no output")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(psgrepout)))
	if err != nil {
		return err
	}
	err = syscall.Kill(pid, syscall.SIGTERM)
	if err != nil {
		return err
	}
	fmt.Printf("kill %d successfully", pid)
	return nil
}

var (
	// HTTP请求总数
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status_code"},
	)

	// HTTP请求持续时间
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	httpRequestErrorTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_error_total",
			Help: "Total number of HTTP request errors",
		},
		[]string{"method", "endpoint", "code"},
	)
)

func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start).Seconds()
		method := c.Request.Method
		endpoint := c.FullPath()
		statusCode := strconv.Itoa(c.Writer.Status())
		httpRequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()

		httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration)

		if c.Writer.Status() >= 400 {
			// 记录错误请求
			code := strconv.Itoa(c.Writer.Status())
			httpRequestErrorTotal.WithLabelValues(method, endpoint, code).Inc()
		}
	}
}
