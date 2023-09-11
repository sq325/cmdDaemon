package main

import (
	"cmdDaemon/config"
	"cmdDaemon/daemon"
	"cmdDaemon/register"
	"cmdDaemon/web/handler"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"context"

	fork "github.com/sevlyar/go-daemon"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	_version = "v4.0 2023-09-08"
)

// flags
var (
	createConfFile   *bool     = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile       *string   = pflag.String("config.file", "./daemon.yml", "Daemon configuration file name.")
	version          *bool     = pflag.BoolP("version", "v", false, "Print version information.")
	port             *string   = pflag.String("web.port", "9090", "Port to listen.")
	consulAddr       *string   = pflag.String("consul.addr", "", "Consul address. e.g. localhost:8500")
	consulIfList     *[]string = pflag.StringSlice("consul.infList", []string{"bond0", "eth0", "eth1"}, `Network interface list. e.g. --consul.infList="v1,v2"`)
	consulSvcRegFile *string   = pflag.String("consul.svcRegFile", "./services.json", "Consul service register file name.")
	logLevel         *string   = pflag.String("log.level", "info", "Log level. e.g. debug, info, warn, error, dpanic, panic, fatal")

	printCmd *bool = pflag.BoolP("printCmd", "p", false, "Print cmds parse from config.")

	killcmds *bool = pflag.Bool("kill", false, "Kill all child processes from config.")
	// printConsulConf *bool = pflag.Bool("printConsulConf", false, "Print consul config.")
)

var (
	conf *config.Conf

	signCh = make(chan os.Signal)
)

func init() {
	pflag.Parse()
	initConf()
}

func main() {
	if *createConfFile {
		createConfigFile()
		return
	}
	if *version {
		fmt.Println(_version)
		return
	}
	if *printCmd {
		cmds := createCmds(conf)
		for _, cmd := range cmds {
			fmt.Println(cmd.String())
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
	var logger *zap.SugaredLogger
	switch *logLevel {
	case "debug":
		logger = createLogger(zapcore.DebugLevel)
	case "info":
		logger = createLogger(zapcore.InfoLevel)
	case "warn":
		logger = createLogger(zapcore.WarnLevel)
	case "error":
		logger = createLogger(zapcore.ErrorLevel)
	default:
		logger = createLogger(zapcore.InfoLevel)
	}
	logger.Debugln("port: ", *port)
	logger.Debugln("consulAddr: ", *consulAddr)

	// config
	cmds := createCmds(conf)
	if len(cmds) == 0 {
		logger.Fatalln("No cmd to run. Daemon existed.")
		return
	}

	// signal
	signal.Notify(signCh, syscall.SIGHUP, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化Daemon
	onceDaemon := sync.OnceValue(func() *daemon.Daemon {
		return createDaemon(ctx, cmds, logger)
	})
	d := onceDaemon()
	logger.Infoln("Daemon created.")
	logger.Debugf("daemon: %+v\n", d.DCmds)
	go d.Run() // run cmds

	// 初始化consul
	time.Sleep(5 * time.Second) // wait for cmds running
	var consul *register.Consul
	logger.Infoln("Consuladdr: ", *consulAddr)
	switch *consulAddr {
	case "":
		onceConsul := sync.OnceValues(func() (*register.Consul, error) {
			return createConsul(*consulAddr, d, *consulIfList, logger)
		})
		consul, err := onceConsul()
		if err != nil {
			logger.Errorln("Create consul failed. ", err)
		}
		logger.Debugf("consul: %+v\n", consul)
		if consul == nil {
			break
		}
		logger.Infoln("Consul created.")
		func() {
			consul.Updatesvclist()
			if err := consul.Register(); err != nil {
				logger.Errorln("Register failed. ", err)
			}
			logger.Infoln("Register successfully.")
		}()
		go func() { // 启watch
			time.Sleep(10 * time.Second)
			consul.Watch()
		}()
		func() {
			svcFile, err := os.OpenFile(*consulSvcRegFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				logger.Errorln("Open service register file failed. ", err)
			} else {
				defer svcFile.Close()
				consul.PrintConf(svcFile)
			}
		}()
	}

	// 初始化handler
	handler := handler.NewHandler(logger, d)
	go handler.Listen(*port) // listen manager web port

	// 防止子进程成为僵尸进程
	defer func() {
		pid := os.Getpid()
		cancel()
		if consul != nil {
			consul.Deregister()
		}
		syscall.Kill(-pid, syscall.SIGTERM)
		time.Sleep(5 * time.Second)
	}()

	// 捕捉信号
	for sig := range signCh {
		switch sig {
		// 重新加载配置文件
		case syscall.SIGHUP:
			// recover initConf panic
			var initConfPanic bool
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("Reload config failed. %v", r)
						logger.Infoln("Panic Recover. Nothing changed.")
						initConfPanic = true
					}
				}()
				initConf()
			}()
			if initConfPanic { // if initConf panic, do not reload
				break
			}

			logger.Infoln("Reloaded config.")
			cmds := config.GenerateCmds(conf)
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
					logger.Errorf("Cmd: %s Pid: %d kill failed. %v", dcmd.Cmd.String(), pid, err)
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
			logger.Infoln("Ctx canceled. All child processes killed.")

			// reload Daemon and run new cmds
			ctx, cancel = context.WithCancel(context.Background())
			d.Reload(ctx, cmds)
			go d.Run()

			// register again
			if consul != nil {
				<-time.After(10 * time.Second)
				if err := consul.RegisterAgain(); err != nil {
					logger.Errorln("Register again failed. err:", err)
				}
				logger.Infoln("Register again successfully.")
				logger.Infoln("Restart completely.")
			}

		// kill all child processes
		case syscall.SIGTERM:
			logger.Warnln("Catched a term sign, kill all child processes. ", time.Now().Format(time.DateTime))
			return // defer 会kill所有子进程
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
func NewLogger(level zapcore.Level) *zap.SugaredLogger {
	writer := os.Stdout
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 时间格式：2020-12-16T17:53:30.466+0800
	// encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder   // 时间格式：2020-12-16T17:53:30.466+0800
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 在日志文件中使用大写字母记录日志级别
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	writeSyncer := zapcore.AddSync(writer)

	core := zapcore.NewCore(encoder, writeSyncer, level)

	logger := zap.New(core, zap.AddCaller()).Sugar() // AddCaller() 显示行号和文件名
	return logger
}
