package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"prometheusDaemon/config"
	"prometheusDaemon/daemon"
	"syscall"
	"time"

	"context"

	fork "github.com/sevlyar/go-daemon"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	_version = "v2.1 2023-03-01"
)

// flags
var (
	createConfFile *bool   = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile     *string = pflag.String("config.file", "./daemon.yml", "Daemon configuration file name.")
	version        *bool   = pflag.BoolP("version", "v", false, "Print version information.")

	printCmd *bool = pflag.BoolP("printCmd", "p", false, "Print cmd to run.")
)

var (
	conf   *config.Conf
	logger *zap.SugaredLogger

	signCh = make(chan os.Signal)
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

	// print cmds
	if *printCmd {
		initConf()
		cmds := config.GenerateCmds(conf)
		for _, cmd := range cmds {
			fmt.Println(cmd.String())
		}
		return
	}

	// 监听SIGHUP和SIGTERM信号
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
	initLogger()
	cmds := config.GenerateCmds(conf)
	if len(cmds) == 0 {
		logger.Fatalln("No cmd to run. Daemon existed.")
		return
	}

	signal.Notify(signCh, syscall.SIGHUP, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	Daemon := daemon.NewDaemon(ctx, cmds, logger)
	go Daemon.Run()

	// 捕捉信号
	for {
		select {
		case sig := <-signCh:
			switch sig {
			case syscall.SIGHUP:
				// 重新加载配置文件, recover initConf panic
				logger.Infoln("Reloading config.")
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
				if initConfPanic {
					break
				}
				cmds := config.GenerateCmds(conf)
				if len(cmds) == 0 {
					logger.Error("No cmd to run. Do not reload.")
					break
				}
				// 关闭所有子进程
				cancel()
				for _, dcmd := range Daemon.DCmds {
					if dcmd.Status == daemon.Exited {
						continue
					}
					pid := dcmd.Cmd.Process.Pid
					err := syscall.Kill(pid, syscall.SIGTERM)
					time.Sleep(time.Second)
					if err != nil {
						logger.Errorf("Cmd: %s Pid: %d kill failed. %v", dcmd.Cmd.String(), pid, err)
					}
				}
				logger.Infoln("Ctx canceled. All child processes killed.")
				// reload Daemon and run new cmds
				ctx, cancel = context.WithCancel(context.Background())
				Daemon.Reload(ctx, cmds)
				go Daemon.Run()
				logger.Infoln("Reload completely.")
			// kill all child processes
			case syscall.SIGTERM:
				logger.Warnln("Catched a term sign, kill all child processes. ", time.Now().Format(time.DateTime))
				cancel()
				pid := os.Getpid()
				syscall.Kill(-pid, syscall.SIGTERM)
				time.Sleep(2 * time.Second)
				return
			}
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
func initLogger() {
	writer := os.Stdout
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 时间格式：2020-12-16T17:53:30.466+0800
	// encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder   // 时间格式：2020-12-16T17:53:30.466+0800
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 在日志文件中使用大写字母记录日志级别
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	writeSyncer := zapcore.AddSync(writer)

	var level = zap.DebugLevel
	core := zapcore.NewCore(encoder, writeSyncer, level)

	logger = zap.New(core, zap.AddCaller()).Sugar() // AddCaller() 显示行号和文件名
}
