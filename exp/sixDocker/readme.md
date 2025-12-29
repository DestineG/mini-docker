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

### 4.1 ./sixDocker run -ti /bin/sh

- 在 3.2 的基础上 NewParentProcess() 中加入了 ```cmd.Dir = "/root/busybox"```用于指定容器进程的工作目录
- 在 RunContainerInitProcess() 中加入了 SetUpMount() 将当前容器进程的工作目录作为其根文件系统

#### tips

- 切换新的文件系统不会自动挂载 /proc、/dev等目录，需要手动挂载
- proc 作为特殊的文件系统，其内容会随着 namespace 切换
- tmpfs 是一种基于内存的特殊文件系统，用于存放临时文件，也常被用来承载 /dev 等目录中的设备节点

### 4.2 ./sixDocker run -ti /bin/sh

- 在 4.1 基础上 NewParentProcess() 加入了aufs文件系统的创建
- 在 Run() 添加了退出容器时对aufs文件系统的卸载以及 writeLayer 的清理

#### tips

- aufs 文件系统创建时依赖的 dirs 不能属于 unionfs，也就是说 aufs 不支持在联合文件系统上嵌套 aufs 文件系统

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