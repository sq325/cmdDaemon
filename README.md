# 介绍

此守护程序根据`config.yml`文件逐个生成`cmd`(`exec.Cmd`)，并和`Limiter`一起封装成`DaemonCmd`对象。`Limiter`用来限制`cmd`的重启间隔和次数，并在一段时间内重置限制。

`Daemon`对象负责管理所有`DaemonCmd`，并发运行它们并监听`exitedCmdCh`通道。当`cmd.Start`报错或`cmd.Wait`退出时，`exitedCmdCh`传递`DaemonCmd`传递给`Daemon`处理。`Daemon`根据重启次数和重启间隔来决定是否重启此`cmd`。

如果接收到`SIGTERM`信号，守护程序将向所有子进程发送`SIGTERM`信号并退出。

如果接收到`SIGHUP`信号，守护进程将执行以下步骤：

1. 尝试重新加载`config.yml`。
2. 释放所有`goroutine`。
3. 向所有子进程发送`SIGTERM`信号。
4. 生成新的`Daemon`对象重新运行新的`DaemonCmds`。
5. 如果`config.yml`有错误，守护进程会继续运行旧的`DaemonCmds`。

感谢`github.com/sevlyar/go-daemon`项目，此守护进程的实现参考了该项目。

## 使用

```bash
make build # 编译
./cmdDaemon --config.createDefault # 生成默认配置文件。需手动添加要启动的cmd
./cmdDaemon # 运行
```
