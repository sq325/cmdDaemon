package main

import (
	"cmdDaemon/config"
	"cmdDaemon/daemon"
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
	_version = "v4.0 2023-09-17"
)

// flags
var (
	createConfFile *bool   = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile     *string = pflag.String("config.file", "./daemon.yml", "Daemon configuration file name.")
	version        *bool   = pflag.BoolP("version", "v", false, "Print version information.")
	port           *string = pflag.String("web.port", "9090", "Port to listen.")
	consulAddr     *string = pflag.String("consulAddr", "", "Consul address. e.g. localhost:8500")

	printCmd        *bool = pflag.BoolP("printCmd", "p", false, "Print cmds parse from config.")
	printConsulConf *bool = pflag.Bool("printConsulConf", false, "Print consul config.")
)

var (
	conf *config.Conf

	signCh      = make(chan os.Signal)
	cmdChangeCh = make(chan struct{}, 20)
)

func init() {
	pflag.Parse()
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
		initConf()
		cmds := config.GenerateCmds(conf)
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

	initConf()
	logger := NewLogger()
	cmds := config.GenerateCmds(conf)
	if len(cmds) == 0 {
		logger.Fatalln("No cmd to run. Daemon existed.")
		return
	}

	signal.Notify(signCh, syscall.SIGHUP, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	// 防止子进程成为僵尸进程
	defer func() {
		pid := os.Getpid()
		cancel()
		syscall.Kill(-pid, syscall.SIGTERM)
		time.Sleep(5 * time.Second)
	}()

	// 初始化Daemon, 单例
	OnceDaemon := struct {
		sync.Once
		Daemon *daemon.Daemon
	}{}
	OnceDaemon.Do(func() {
		OnceDaemon.Daemon = daemon.NewDaemon(ctx, cmds, logger)
	})

	go OnceDaemon.Daemon.Run() // run cmds

	// 初始化handler
	handler := handler.NewHandler(logger, OnceDaemon.Daemon)
	go handler.Listen(*port) // listen manager web port

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
			for _, dcmd := range OnceDaemon.Daemon.DCmds {
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
			OnceDaemon.Daemon.Reload(ctx, cmds)
			go OnceDaemon.Daemon.Run()
			logger.Infoln("Restart completely.")

		// kill all child processes
		case syscall.SIGTERM:
			logger.Warnln("Catched a term sign, kill all child processes. ", time.Now().Format(time.DateTime))
			return // defer 会kill所有子进程
		}
	}
}

// 守护进程上下文
func newForkCtx() *fork.Context {
	commandName := os.Args[0]
	return &fork.Context{
		PidFileName: "daemon.pid",
		PidFilePerm: 0644,
		LogFileName: "daemon.log",
		LogFilePerm: 0644,
		WorkDir:     "./",
		Umask:       027,
		Args:        []string{commandName},
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
func NewLogger() *zap.SugaredLogger {
	writer := os.Stdout
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 时间格式：2020-12-16T17:53:30.466+0800
	// encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder   // 时间格式：2020-12-16T17:53:30.466+0800
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 在日志文件中使用大写字母记录日志级别
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	writeSyncer := zapcore.AddSync(writer)

	var level = zap.DebugLevel
	core := zapcore.NewCore(encoder, writeSyncer, level)

	logger := zap.New(core, zap.AddCaller()).Sugar() // AddCaller() 显示行号和文件名
	return logger
}
