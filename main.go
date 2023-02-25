// PrometheusDaemon is a daemon for several prometheus instances
// Functions:
// 1. Run prometheus instances according to prometheusDaemon.yml
// 2. Restart instances existed with errors
// 3. generate all prometheus instances scrape job config
// 4. kill all instances while daemon process was killed (pgid)

// 不处理具体业务逻辑，只是再次按一样的参数调用自身，启动一个子进程，有子进程负责业务逻辑处理。守护进程监视子进程状态，若退出则再次启动一次。

package main

import (
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"prometheusDaemon/config"
	"strconv"
	"syscall"
	"time"

	fork "github.com/sevlyar/go-daemon"
	"github.com/spf13/pflag"
)

var (
	createConfFile *bool   = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile     *string = pflag.String("config.file", "./prometheusDaemon.yml", "prometheusDaemon configuration file name.")
)

var (
	conf        *config.Conf
	exitedCmdCh = make(chan *exec.Cmd, 20)
	runningCmds = make(map[string]*exec.Cmd, 20)
	done        = make(chan struct{}, 1)
	signCh      = make(chan os.Signal)
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
	// 监听SIGHUP和SIGTERM信号
	signal.Notify(signCh, syscall.SIGHUP, syscall.SIGTERM)
	cntxt := newForkCtx()
	child, err := cntxt.Reborn()
	if err != nil {
		log.Fatal("Unable to run: ", err)
	}
	if child != nil { // 如果此时在父进程中，直接退出
		return
	}
	defer cntxt.Release() // 在函数结束时重启stdin、stdout、stderr

	log.Print("- - - - - - - - - - - - - - -")
	log.Printf("prometheusDaemon started %s", time.Now().Format(time.DateTime))

	cmds := []*exec.Cmd{
		exec.Command("./subapp", "-n", "1", "-i", "3s"),
		exec.Command("./subapp", "-n", "2", "-i", "3s"),
		exec.Command("./subapp", "-n", "3", "-i", "3s"),
	}

	for _, cmd := range cmds {
		go runCmd(cmd) // exitedCmdCh 生产者
	}

	for {
		select {
		// 接收exitedCmdCh中需要restart的cmd
		case cmd, ok := <-exitedCmdCh: // exitedCmdCh 消费者
			if !ok {
				log.Println("exitedCmdCh is closed")
				return
			}
			log.Println("Cmd is exited: ", cmd.String())
			log.Println("Exitcode: ", cmd.ProcessState.ExitCode())
			cmdSlice := cmd.Args
			newCmd := exec.Command(cmdSlice[0], cmdSlice[1:]...)

			log.Println("Restarting cmd: ", newCmd.String())
			go runCmd(newCmd)
		// 控制
		case <-done:
			log.Println("Receive Done sig, exist.")
			return
		// 捕捉信号
		case sig := <-signCh:
			switch sig {
			case syscall.SIGHUP:
				log.Println("Reloading configs.")
				reload()
			case syscall.SIGTERM:
				log.Println("Catched a term sign, kill all child processes. ", time.Now().Format(time.DateTime))
				pid := os.Getpid()
				syscall.Kill(-pid, syscall.SIGTERM)
				return
			}
		}
	}
}

// 运行cmd
func runCmd(cmd *exec.Cmd) {
	cmd.Start()
	key := hashCmd(cmd)
	runningCmds[key] = cmd
	err := cmd.Wait()
	if err != nil {
		log.Printf("%s start err: %s", cmd.String(), err)
		return
	}
	exitedCmdCh <- cmd
}

// 生成cmd的hash值
func hashCmd(cmd *exec.Cmd) string {
	hash := fnv.New32()
	cmdstr, pid := cmd.String(), cmd.Process.Pid
	cmdstrpid := cmdstr + strconv.Itoa(pid)
	hash.Write([]byte(cmdstrpid))
	return strconv.Itoa(int(hash.Sum32()))
}

func reload() {

}

// 守护进程上下文
func newForkCtx() *fork.Context {
	return &fork.Context{
		PidFileName: "daemon.pid",
		PidFilePerm: 0644,
		LogFileName: "daemon.log",
		LogFilePerm: 0644,
		WorkDir:     "./",
		Umask:       027,
		Args:        []string{"prometheusDaemon"},
	}
}

// 生成默认配置文件
func createConfigFile() {
	str := config.DefaultConfig
	_, err := os.Stat("prometheusDaemon.yml")
	if err == nil || os.IsExist(err) {
		fmt.Println("prometheusDaemon.yml existed")
		return
	}
	file, err := os.Create("prometheusDaemon.yml")
	if err != nil {
		fmt.Println("create config.yml file err:", err)
		return
	}
	file.WriteString(str)
	fmt.Println("prometheusDaemon.yml created.")
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
