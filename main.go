package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sq325/cmdDaemon/config"
	"github.com/sq325/cmdDaemon/daemon"
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
	ginSwagger "github.com/swaggo/gin-swagger"
)

var (
	projectName    string
	_versionInfo   string
	buildTime      string
	buildGoVersion string
	_version       string
	author         string
)

// flags
var (
	createConfFile *bool   = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile     *string = pflag.String("config.file", "./daemon.yml", "Daemon configuration file name.")
	version        *bool   = pflag.BoolP("version", "v", false, "Print version information.")
	port           *string = pflag.String("web.port", "9090", "Port to listen.")
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
		fmt.Println(projectName, _version)
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
	mux := gin.New() // Changed from gin.Default()
	// gin.SetMode(gin.ReleaseMode) // Already default in gin.New() if not debug
	mux.Use(gin.Recovery()) // Add Recovery middleware if needed, Default() includes it

	mux.Use(prometheusMiddleware())

	mux.PUT("/restart", func(c *gin.Context) {
		// Check for the 'update' query parameter
		if _, ok := c.GetQuery("update"); ok {
			logger.Info("Update requested before reload")
			if err := svc.Update(); err != nil {
				logger.Error("Service update failed during restart sequence", "error", err)
				c.JSON(500, handler.SvcManagerResponse{Err: "update failed: " + err.Error()})
				return
			}
			logger.Info("Service updated successfully before restart")
		}

		if err := svc.Restart(); err != nil {
			logger.Error("Service restart failed", "error", err)
			c.JSON(500, handler.SvcManagerResponse{Err: "restart failed: " + err.Error()})
			return
		}
		logger.Info("Service restarted successfully")
		c.JSON(200, handler.SvcManagerResponse{V: "ok"})
	})

	mux.PUT("/reload", func(c *gin.Context) {
		// Check for the 'update' query parameter
		if _, ok := c.GetQuery("update"); ok {
			logger.Info("Update requested before restart")
			if err := svc.Update(); err != nil {
				logger.Error("Service update failed during restart sequence", "error", err)
				c.JSON(500, handler.SvcManagerResponse{Err: "update failed: " + err.Error()})
				return
			}
			logger.Info("Service updated successfully before restart")
		}

		err := svc.Reload()
		if err != nil {
			c.JSON(500, handler.SvcManagerResponse{Err: err.Error()})
			return
		}
		c.JSON(200, handler.SvcManagerResponse{V: "ok"})
	})

	mux.Any("/list", func(c *gin.Context) {
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
				c.String(200, "%s service is healthy", projectName)
				return
			}
			c.String(500, "%s service is unhealthy", projectName)
		},
	)
	mux.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	mux.GET("/metrics", gin.WrapH(metricsHandler))
	mux.GET("/discovery", gin.WrapH(daemon.HttpSDHandler(d)))

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
