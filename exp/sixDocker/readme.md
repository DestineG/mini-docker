## 执行流

### ./sixDocker run -ti /bin/sh
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

## 小记

- 所有命令行参数会在主进程中解析完毕
- cli程序的 选项参数 使用cli.Flag定义，使用 -或--传递，使用context.Bool访问
- cli程序的 位置参数 使用context.Args访问