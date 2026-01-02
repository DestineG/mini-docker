# sixDocker

一个用 Go 语言实现的简易容器运行时，用于学习和理解 Docker 的工作原理。

## 📖 项目简介

sixDocker 是一个容器运行时实现演示项目，旨在帮助开发者深入理解容器技术的核心原理。项目实现了类似 Docker 的基本功能，包括容器创建、资源限制、网络管理、镜像管理等核心特性。

## ✨ 功能特性

### 容器管理
- ✅ **创建和运行容器** - 支持前台和后台运行模式
- ✅ **容器生命周期管理** - 启动、停止、删除容器
- ✅ **容器列表查看** - 查看所有容器的状态信息
- ✅ **容器日志** - 查看容器的标准输出和错误输出
- ✅ **容器执行命令** - 在运行中的容器内执行命令

### 资源限制
- ✅ **内存限制** - 通过 Cgroups 限制容器内存使用
- ✅ **CPU 限制** - 支持 CPU 份额和 CPU 集合限制
- ✅ **CPU 集合** - 指定容器可使用的 CPU 核心

### 文件系统
- ✅ **Union File System** - 实现类似 Docker 的镜像分层存储
- ✅ **卷挂载** - 支持将宿主机目录挂载到容器内
- ✅ **镜像管理** - 支持容器提交为镜像，查看所有镜像

### 网络功能
- ✅ **网络创建** - 支持创建 Bridge 网络
- ✅ **网络管理** - 网络列表查看和删除
- ✅ **端口映射** - 支持容器端口映射到宿主机
- ✅ **容器网络连接** - 容器可以连接到指定网络

### 其他功能
- ✅ **环境变量** - 支持设置容器环境变量
- ✅ **交互式终端** - 支持 TTY 模式，提供交互式体验

## 🛠️ 技术实现

### Linux Namespace
- **UTS Namespace** - 隔离主机名和域名
- **PID Namespace** - 隔离进程 ID
- **Mount Namespace** - 隔离文件系统挂载点
- **Network Namespace** - 隔离网络设备、端口等
- **IPC Namespace** - 隔离进程间通信

### Cgroups
- 使用 Cgroups v1 实现资源限制
- 支持 Memory、CPU、CPUSet 子系统

### Union File System
- 实现类似 AUFS 的联合文件系统
- 支持只读层和可写层的分离
- 实现 Copy-on-Write (COW) 机制

### 网络实现
- 使用 Linux Bridge 实现容器网络
- 通过 veth pair 连接容器和宿主机
- 使用 iptables 实现 NAT 和端口映射
- 实现 IPAM (IP Address Management) 管理容器 IP

## 📋 系统要求

- Linux 操作系统（推荐 Ubuntu/Debian）
- Go 1.22.4 或更高版本
- Root 权限（容器运行时需要）
- 已安装并启用 Cgroups
- 已安装 iptables（用于网络功能）

## 🚀 安装和编译

### 1. 克隆项目

```bash
git clone <repository-url>
cd sixDocker
```

### 2. 安装依赖

```bash
go mod download
```

### 3. 编译

```bash
go build -o sixDocker .
```

### 4. 准备镜像

项目主要使用的镜像有两个 busybox nginx，可以在 exp/images 目录下拿到 tar 文件，将其放到 `/var/run/sixDocker/images/` 目录下
也可以自行在 `container_process.go` 中配置镜像存放目录

## 📚 使用示例

### 基本使用

#### 1. 运行一个交互式容器

```bash
sudo ./sixDocker run -ti --name mycontainer -- /bin/sh
```

#### 2. 运行后台容器

```bash
sudo ./sixDocker run -d --name mycontainer -- top
```

#### 3. 查看容器列表

```bash
sudo ./sixDocker ps
```

#### 4. 查看容器日志

```bash
sudo ./sixDocker logs mycontainer
```

#### 5. 在容器内执行命令

```bash
sudo ./sixDocker exec mycontainer /bin/sh
```

#### 6. 停止容器

```bash
sudo ./sixDocker stop mycontainer
```

#### 7. 删除容器

```bash
sudo ./sixDocker rm mycontainer
```

### 资源限制

#### 限制内存使用

```bash
sudo ./sixDocker run -ti -m 100m --name test -- stress --vm-bytes 200m --vm-keep -m 1
```

#### 限制 CPU 份额

```bash
sudo ./sixDocker run -ti -cpushare 512 --name test -- /bin/sh
```

#### 限制 CPU 集合

```bash
sudo ./sixDocker run -ti -cpuset 0,1 --name test -- /bin/sh
```

### 卷挂载

```bash
sudo ./sixDocker run -ti -v /host/path:/container/path:rw --name test -- /bin/sh
```

### 网络功能

#### 创建网络

```bash
sudo ./sixDocker network create -driver bridge -subnet 172.18.0.0/16 docker0
```

#### 查看网络列表

```bash
sudo ./sixDocker network list
```

#### 运行容器并连接到网络

```bash
sudo ./sixDocker run -d --name web -network docker0 -p 80:80 -- nginx -g 'daemon off;'
```

#### 删除网络

```bash
sudo ./sixDocker network remove docker0
```

### 镜像管理

#### 提交容器为镜像

```bash
sudo ./sixDocker commit -n mycontainer -t myimage
```

#### 查看所有镜像

```bash
sudo ./sixDocker images
```

#### 使用指定镜像运行容器

```bash
sudo ./sixDocker run -ti -image nginx --name web -- nginx -g 'daemon off;'
```

### 环境变量

```bash
sudo ./sixDocker run -d -e KEY1=value1 -e KEY2=value2 --name test -- /bin/sh
```

## 📖 命令说明

### run

创建并运行一个新容器。

```bash
./sixDocker run [OPTIONS] -- COMMAND [ARG...]
```

**选项：**
- `-ti` - 启用交互式终端
- `-d` - 后台运行容器
- `-name` - 指定容器名称
- `-m` - 内存限制（如：100m, 1g）
- `-cpushare` - CPU 份额限制
- `-cpuset` - CPU 集合（如：0,1）
- `-v` - 卷挂载（格式：host:container:mode）
- `-e` - 环境变量（格式：KEY=value）
- `-network` - 网络名称（默认：bridge）
- `-p` - 端口映射（格式：host:container）
- `-image` - 镜像名称（默认：busybox）

### ps

列出所有容器。

```bash
./sixDocker ps
```

### logs

查看容器日志。

```bash
./sixDocker logs CONTAINER_NAME
```

### exec

在运行中的容器内执行命令。

```bash
./sixDocker exec CONTAINER_NAME COMMAND [ARG...]
```

### stop

停止一个运行中的容器。

```bash
./sixDocker stop CONTAINER_NAME
```

### rm

删除一个容器。

```bash
./sixDocker rm CONTAINER_NAME
```

### commit

将容器提交为镜像。

```bash
./sixDocker commit -n CONTAINER_NAME -t IMAGE_NAME
```

### images

列出所有镜像。

```bash
./sixDocker images
```

### network

网络管理命令。

```bash
# 创建网络
./sixDocker network create -driver bridge -subnet SUBNET NETWORK_NAME

# 列出网络
./sixDocker network list

# 删除网络
./sixDocker network remove NETWORK_NAME
```

## 📁 项目结构

```
sixDocker/
├── main.go                 # 主入口文件
├── main_command.go         # CLI 命令定义
├── run.go                  # 容器运行逻辑
├── container/              # 容器相关功能
│   ├── container_process.go # 容器进程管理
│   ├── init.go             # 容器初始化
│   └── exec.go             # exec 命令实现
├── cgroups/                # Cgroups 资源限制
│   ├── cgroup_manager.go   # Cgroup 管理器
│   └── subsystems/         # 各子系统实现
│       ├── memory.go       # 内存限制
│       ├── cpu.go          # CPU 限制
│       └── cpuset.go       # CPU 集合
├── network/                # 网络功能
│   ├── network.go          # 网络管理
│   ├── bridge.go           # Bridge 网络实现
│   └── ipam.go             # IP 地址管理
├── nsenter/                # Namespace 进入
│   └── nsenter.go          # CGO 实现的 setns
└── go.mod                  # Go 模块定义
```

## ⚠️ 注意事项

1. **需要 Root 权限**：容器运行时需要 root 权限来创建 Namespace 和操作 Cgroups。

2. **镜像准备**：使用前需要手动准备容器镜像文件系统，放置在 `/var/run/sixDocker/readOnlyLayer/` 目录下。

3. **资源清理**：后台运行的容器在停止后需要手动清理资源，某些情况下可能需要手动卸载挂载点。

4. **网络限制**：网络功能需要 iptables 支持，某些系统可能需要额外配置。

5. **实验性质**：这是一个学习项目，不建议在生产环境使用。

6. **系统兼容性**：主要针对 Linux 系统，需要内核支持 Namespace 和 Cgroups。

## 🔧 依赖项

- `github.com/urfave/cli` - CLI 框架
- `github.com/sirupsen/logrus` - 日志库
- `github.com/vishvananda/netlink` - 网络管理
- `github.com/vishvananda/netns` - Network Namespace 操作
- `github.com/olekukonko/tablewriter` - 表格输出

## 📝 开发说明

本项目主要用于学习和理解容器技术原理。详细的技术实现和执行流程请参考项目中的 `sixDocker/readme.md` 文件。

## 🙏 致谢

本项目参考了[《自己动手写 docker》](https://github.com/xianlubird/mydocker)。

