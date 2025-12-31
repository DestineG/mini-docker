## 执行流

### 3.1 ./sixDocker run -ti /bin/sh

``` bash
run            (PID = A)
 ├─ main()
 ├─ runCommand() (-ti /bin/sh -> tty, [command])
 └─ Run()
     └─ NewParentProcess() (tty: 将系统交互和创建的rootContainer进程关联)

clone (new namespaces, PID = B)
exec  /proc/self/exe init [command]

init            (PID = B, 容器 PID 1)
 ├─ main()
 ├─ initCommand()
 └─ RunContainerInitProcess()
      └─ exec /bin/sh
```

#### tips

- 所有命令行参数会在主进程中解析完毕
- cli程序的 选项参数 使用cli.Flag定义，使用 -或--传递，使用context.Bool访问
- cli程序的 位置参数 使用context.Args访问

---

### 3.2 ./sixDocker run -ti -m 100m -- stress --vm-bytes 800m --vm-keep -m 1

``` bash
父进程：sixDocker run                      (PID = A)

Main()
 └─ run → Run()
     ├─ NewParentProcess()
     │   ├─ 创建 pipe (read, write)
     │   ├─ 构造 exec.Cmd (/proc/self/exe init)
     │   ├─ 设置 namespace (UTS / PID / NS / NET / IPC)
     │   └─ 预置 ExtraFiles → 子进程 fd=3 (pipe 读端)
     │
     ├─ Start()
     │   └─ clone + exec → 生成 init 子进程
     │
     ├─ NewCgroupManager()
     │   └─ 创建/定位 cgroup 目录
     │
     ├─ Set()
     │   └─ 写入资源限制参数 (memory / cpu / cpuset)
     │
     ├─ Apply()
     │   └─ 将 init 子进程 PID 加入 cgroup
     │
     └─ sendInitCommand()
         ├─ 通过 pipe 写端发送用户命令
         └─ 关闭写端（通知 EOF）

────────────────────────────────────────────────────────

子进程：sixDocker init                    (PID = B, 容器 PID = 1)

Main()
 └─ init → RunContainerInitProcess()
     ├─ readUserCommand()
     │   ├─ 从 fd=3 读取 pipe 数据
     │   ├─ 若父进程未写入 → 阻塞
     │   └─ 直到父进程关闭写端 → 读取完成
     │
     ├─ exec.LookPath()
     │   └─ 在当前 mount namespace + PATH 中查找可执行文件
     │
     └─ syscall.Exec()
         └─ 用用户程序替换 init 进程映像
```

#### tips

- cli程序对命令行参数的解析遇到 ```--``` 就会终止，在它之前的Flag会全部尝试解析，如果传入未定义的Flag就会报错，在它之后的参数会放到字符串列表context.Args()
- 父子进程通信
     * `cmd.ExtraFiles` 会在 **父进程调用 `Start()` 时**，先被加入到**父进程的文件描述符表**中，并从 **fd=3 开始编号**
     * 子进程在 `clone + exec` 后 **继承父进程的文件描述符表**
     * 因此子进程可通过 `os.NewFile(uintptr(3), ...)` 直接访问父进程传入的 pipe
     * 该方式依赖文件描述符继承，可安全跨 `exec`，常用于容器 init 进程通信

---

### 4.1 ./sixDocker run -ti /bin/sh

- 在 3.2 的基础上 NewParentProcess() 中加入了 ```cmd.Dir = "/root/busybox"```用于指定容器进程的工作目录
- 在 RunContainerInitProcess() 中加入了 SetUpMount() 将当前容器进程的工作目录作为其根文件系统

#### tips

- 切换新的文件系统不会自动挂载 /proc、/dev等目录，需要手动挂载
- proc 作为特殊的文件系统，其内容会随着 namespace 切换
- tmpfs 是一种基于内存的特殊文件系统，用于存放临时文件，也常被用来承载 /dev 等目录中的设备节点

---

### 4.2 ./sixDocker run -ti /bin/sh

- 在 4.1 基础上 NewParentProcess() 加入了aufs文件系统的创建
- 在 Run() 添加了退出容器时对aufs文件系统的卸载以及 writeLayer 的清理

#### tips

- aufs 文件系统创建时依赖的 dirs 不能属于 unionfs，也就是说 aufs 不支持在联合文件系统上嵌套 aufs 文件系统

---

### 4.3

- 测试命令: `./sixDocker run -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume0:/tmp/v0:rw -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume1:/tmp/v1:ro -ti -- sh`
- 添加了 volume 功能：v 参数解析 -> NewWorkSpace() 使用
- 添加 volume 的卸载

#### tips

- volume 实现
    - 创建 ufs
    - 将宿主机目录通过 mount 挂载到 ufs 中指定目录
- 带有 volume 的 ufs 卸载，需要先卸载 volume 才能卸载 ufs(位置: DeleteMountPoint)

- COW(copy on write) 的触发机制
    - ufs = upperdir(mount 时的dirs[0]) + lowdir(mount 时的dirs[1:])
    - 写操作 + 目标文件位于 lowerdir

---

### 4.4

- 测试命令: 
    - `./sixDocker run -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume0:/tmp/v0:rw -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume1:/tmp/v1:ro -ti -- sh`
    - `echo "hello commit" > /tmp/commit_test.a`
    - `./sixDocker commit -imageName testbusyBox`
- 添加了 commit 子命令：imageName 参数解析 -> CommitContainer() 使用

#### tips

- go 的 package 中的函数的可见性由函数名首字母决定，大写表示可跨包，小写表示不能跨包

### 5.1

- 添加对 -d 参数的解析(本节代码如果使用了 -d 参数，可能会导致容器资源回收失败(详情见Run()))

#### tips

- 父进程结束而其子进程还在运行时，子进程会被pid=1的进程收养

### 5.2

- 添加对 ps 子命令的支持

#### tips

- 卸载异常挂载点：
    - `umount -l /workspace/projects/go/dockerDev/unionfs/aufs/busybox/mnt`
    - `rm -rf /workspace/projects/go/dockerDev/unionfs/aufs/busybox/mnt`
    - `rm -rf /workspace/projects/go/dockerDev/unionfs/aufs/busybox/writeLayer`

### 5.3

- 添加对 logs 子命令的支持
- 测试命令：
    - `./sixDocker run -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume0:/tmp/v0:rw -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume1:/tmp/v1:ro -d -name sixHelloeverybody -- top`
    - `./sixDocker logs -containerId <容器id>`

#### tips

- 整理依赖树并下载 & 整理
    - `go mod tidy`
    - `go mod vendor`

---

### 5.4 exec 子命令实现

#### 是什么

exec 子命令允许用户在已经运行的容器内部启动一个新的进程。它不同于 run（创建一个新容器），它的核心是在宿主机上启动一个进程，然后通过 Linux 的 setns 系统调用，“潜入”到目标容器的各个隔离命名空间（Namespace）中，然后在新的 Namespace 中执行命令。

#### 怎么做（融入拦截机制的执行流）

```bash
阶段一：宿主机环境 (初次执行没有 env 不会执行 C 拦截逻辑 -> Go 逻辑)
./sixDocker exec <containerId> <command>         (PID = A)
 ├─ main() 解析 exec 子命令
 ├─ 获取目标容器 PID (从 /var/run/sixDocker/<id>/config.json 读取)
 └─ 设置环境变量: 
      export sixDocker_pid=<容器PID>
      export sixDocker_cmd=<用户命令>
    调用 /proc/self/exe (再次启动自己) ──┐
                                       │
───────────────────────────────────────┼────────────────
阶段二：时空穿梭 (C 拦截逻辑)             │
子进程启动 (准备初始化 Go Runtime)        (PID = B) <──┘
 ├─ [ 拦截点 ]：ELF 加载器调用 .init_array (C Constructor)
 ├─ enter_namespace() 开始运行
 │   ├─ getenv("sixDocker_pid") -> 命中！
 │   ├─ 依次 open("/proc/<PID>/ns/...") 获得 5 大 Namespace 句柄
 │   ├─ syscall.setns() -> 进程 B 的视角瞬间切换至容器内部
 │   │    └─ 注意：此时进程依然是单线程，满足内核 setns 强制要求
 │   ├─ system(sixDocker_cmd) -> 在容器内启动目标程序 (切换 namespace 后，会在容器内部寻找可执行文件进行执行)
 │   └─ exit(0) -> 任务完成，直接在 C 阶段自杀
 └─ (Go Runtime 永远没有机会在进程 B 中启动)

```

#### 为什么：深层拦截原理

1. 使用 CGO 的根本原因 Linux 内核为了保证文件系统的安全性，严格规定：不允许在多线程环境下切换 Mount Namespace。由于 Go 语言天生就是多线程（启动即有调度器和 GC 线程），直接在 Go 代码里调用 setns 会被内核直接拒绝（报错 EINVAL）。

2. 抢跑机制 (Pre-Runtime Execution) Go 的 main 函数并不是程序的第一行代码。Linux 加载二进制文件后，会先执行 CGO 产生的构造函数。此时，Go 的多线程调度器（Scheduler）还未出生。我们利用这个单线程的真空期，完成了 Namespace 的切换。

3. 环境变量的“接力棒” 进程 A (Go) 无法直接通过函数参数告诉进程 B (C 阶段) 目标 PID，因为 C 构造函数运行在参数解析之前。环境变量存储在进程的栈顶，是父子进程间最原始、最直接的信息传递方式，C 代码通过 getenv 可以在任何逻辑执行前拿到它。

4. 防止“逻辑污染” 为什么不让进程 B 继续跑 Go 代码？因为进程 B 此时已经进入了容器的 mnt 空间，它看到的 /lib、/etc 全是容器的（可能是 Busybox 或 Alpine）。如果让 Go Runtime 继续初始化，它会因为找不到宿主机的动态库或配置文件而崩溃。因此，exit(0) 是必须的，确保“特工”进程完成任务后立即消失，不留下任何副作用。

#### 测试

- 创建后台容器：`./sixDocker run -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume0:/tmp/v0:rw -v /workspace/projects/go/dockerDev/unionfs/aufs/busybox/volumes/volume1:/tmp/v1:ro -d -name sixHelloeverybody -- top`
- 查看后台容器的id：`./sixDocker ps`
- 进入后台容器执行命令：`./sixDocker exec <容器id> /bin/sh`
- 查看 top 进程：`ps`


exec 交互终端输出展示
``` bash
root@78c966f22b74:/workspace/projects/go/dockerDev/exp/sixDocker# ./sixDocker exec 4267836795 /bin/sh
INFO[0000] main - os.Args: [./sixDocker exec 4267836795 /bin/sh] 
INFO[0000] container pid 172800                         
INFO[0000] exec command /bin/sh                         
/ # ps
PID   USER     TIME  COMMAND
    1 root      0:03 top
   13 root      0:00 /bin/sh
   14 root      0:00 ps
/ # exit
root@78c966f22b74:/workspace/projects/go/dockerDev/exp/sixDocker# 
```

#### tips

- /proc/self/exe：这是一个神奇的符号链接，指向当前正在运行的程序本身。使用它来 re-exec 可以保证父子进程使用的是同一个二进制文件，从而确保子进程里一定带有那段 C 拦截代码，这是实现“自启动拦截”的基础
- fd 的清理：在 C 代码中 setns 成功后，必须及时关闭打开的 /proc/[pid]/ns/* 文件描述符。如果不关闭，这些打开的句柄会泄露到容器内部的 sh 进程中，给容器留下可以直接操作宿主机 Namespace 的“后门”
- 标准输入输出：为了支持交互（-ti），父进程 A（宿主机端）在 re-exec 启动子进程 B 时，必须通过 `cmd.Stdin = os.Stdin`、`cmd.Stdout = os.Stdout` 等方式，将当前终端的控制权透传给子进程。否则，你进入容器后将无法输入任何命令，也看不到任何输出
- CGO Namespace 切换顺序：Mount ns 切换需要在最后，因为前面的 ns 切换需要使用宿主文件系统中 /proc 中的文件，如果提前切换 Mount ns ，可能会导致找不到正确的 `/proc/%s/ns`
- 进程的 Pid 在创建时就固定下来，不会因为切换了 namespace 就改变；父进程(docker exec的go逻辑)虽然切换到了容器的 namespace ，但是其 Pid 不会随之改变，仍然属于宿主机的 pid namespace；子进程(docker exec的CGO逻辑)是在容器的 namespace 环境下创建，所以可以看到其 Pid 属于容器的 pid namespace

---

### 5.7 重置

#### Todo

- 实现 images 子命令
- 容器根文件系统分开管理
- 创建容器时 保证 containerName 的唯一性
- 命令行参数重置，将 containerId 参数换为 containerName

### 5.8 添加 环境变量参数 -e & 更新 exec 子命令

- 在 run 子命令中添加了参数 e 的解析
- exec 子命令在容器内产生的进程不知为何没有继承 init 进程(容器的根进程)的环境变量，所以在 cmd 中添加了缺失的环境变量(`ExecContainer() -> cmd.Env = finalEnv`)