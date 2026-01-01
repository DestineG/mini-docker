// exp/sixDocker/network/network.go

package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sixDocker/container"
	"strings"
	"text/tabwriter"

	"github.com/vishvananda/netns"

	log "github.com/sirupsen/logrus"

	"github.com/vishvananda/netlink"
)

var (
	defaultNetworkPath = "/var/run/sixDocker/network"
	drivers            = map[string]NetworkDriver{}
	networks           = map[string]*Network{}
)

type Network struct {
	Name    string
	IpRange *net.IPNet // 子网网段，包含网关 IP
	Driver  string
}

type Endpoint struct {
	ID          string           `json:"id"`
	Device      netlink.Veth     `json:"dev"`
	IPAddress   net.IP           `json:"ip"`
	MacAddress  net.HardwareAddr `json:"mac"`
	Network     *Network         `json:"network"`
	PortMapping []string         `json:"portmapping"`
}

type NetworkDriver interface {
	Name() string
	Create(subnet string, name string) (*Network, error)
	Delete(network *Network) error
	Connect(network *Network, endpoint *Endpoint) error
	Disconnect(network *Network, endpoint *Endpoint) error
}

func (nw *Network) dump(dumpDir string) error {
	// 创建 Network 存储目录
	if _, err := os.Stat(dumpDir); err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(dumpDir, 0644)
		} else {
			return err
		}
	}

	// 创建 Network 存储文件(文件存在就清空内容)
	nwPath := path.Join(dumpDir, nw.Name)
	nwFile, err := os.OpenFile(nwPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer nwFile.Close()

	// 序列化 Network 对象到 JSON
	nwJson, err := json.Marshal(nw)
	if err != nil {
		return err
	}

	// JSON 写入文件
	_, err = nwFile.Write(nwJson)
	if err != nil {
		return err
	}
	return nil
}

func (nw *Network) remove(dumpDir string) error {
	nwPath := path.Join(dumpDir, nw.Name)
	//文件不存在直接返回
	if _, err := os.Stat(nwPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}
	return os.Remove(nwPath)
}

func (nw *Network) load(dumpPath string) error {
	// 打开存储 Network 信息的文件
	nwFile, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer nwFile.Close()

	// 读取文件内容到内存
	nwJson := make([]byte, 2000)
	n, err := nwFile.Read(nwJson)
	if err != nil {
		return err
	}

	// 用 JSON 将内容反序列化到 Network 对象
	if err := json.Unmarshal(nwJson[:n], nw); err != nil {
		return err
	}
	return nil
}

func Init() error {
	// 网络驱动注册
	var nw = BridgeNetworkDriver{}
	drivers[nw.Name()] = &nw

	// 确保网络存储目录存在
	if _, err := os.Stat(defaultNetworkPath); err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(defaultNetworkPath, 0644)
		} else {
			return err
		}
	}

	// 恢复已有网络
	files, err := os.ReadDir(defaultNetworkPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		// 只要文件，跳过所有的子目录（如 ipam 目录）
		if file.IsDir() {
			continue
		}

		// 获取网络名称（文件名）
		nwName := file.Name()
		nwPath := path.Join(defaultNetworkPath, nwName)

		// 创建 Network 对象并尝试加载
		nw := &Network{
			Name: nwName,
		}

		if err := nw.load(nwPath); err != nil {
			// 容错处理：如果某个文件解析失败，打印日志并继续处理下一个，不中断流程
			log.Errorf("Error loading network file %s: %v", nwPath, err)
			continue
		}

		// 加载到内存 map 中
		networks[nw.Name] = nw
	}

	log.Infof("Init network success!")
	return nil
}

func CreateNetwork(driver, subnet, name string) error {
	log.Infof("Create network %s with driver %s and subnet %s", name, driver, subnet)
	// 分配网关 IP
	_, cidr, err := net.ParseCIDR(subnet)
	ip, err := ipAllocator.Allocate(cidr)
	if err != nil {
		return err
	}
	cidr.IP = ip

	// 创建网络
	nw, err := drivers[driver].Create(cidr.String(), name)
	if err != nil {
		return err
	}

	// 将网络分别保存在内存和磁盘中
	networks[nw.Name] = nw
	return nw.dump(defaultNetworkPath)
}

func ListNetwork() {
	// 初始化 tabwriter
	w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
	fmt.Fprint(w, "NAME\tIP RANGE\tDRIVER\n")

	// 遍历所有网络并打印
	for _, nw := range networks {
		fmt.Fprintf(w, "%s\t%s\t%s\n", nw.Name, nw.IpRange.String(), nw.Driver)
	}

	// 刷新输出
	if err := w.Flush(); err != nil {
		log.Errorf("List network error: %v", err)
	}
}

func DeleteNetwork(nwName string) error {
	// 获取网络对象
	nw, ok := networks[nwName]
	if !ok {
		return fmt.Errorf("no such network %s", nwName)
	}

	// 释放网关 IP
	if err := ipAllocator.Release(nw.IpRange, &nw.IpRange.IP); err != nil {
		return err
	}

	// 删除网络
	if err := drivers[nw.Driver].Delete(nw); err != nil {
		return err
	}

	// 从内存和磁盘中删除网络
	delete(networks, nw.Name)

	if err := nw.remove(defaultNetworkPath); err != nil {
		return err
	}
	log.Infof("Delete network success!")
	return nil
}

// 将当前线程切换到容器的 network namespace
// 返回一个函数以切换回原有的 namespace
// 执行一些操作...后调用返回的函数以切换回原有的 namespace
func enterContainerNetns(enLink *netlink.Link, cinfo *container.ContainerInfo) func() {
	// 打开容器的 network namespace 文件
	f, err := os.OpenFile(fmt.Sprintf("/proc/%s/ns/net", cinfo.Pid), os.O_RDONLY, 0)
	if err != nil {
		log.Errorf("open netns file error: %v", err)
		return nil
	}

	// 拿到文件描述符
	nsFD := f.Fd()

	// network namespace 是线程级别的 当前线程切换到容器的 netns 不代表其他线程也会切换
	// 对 network namespace 操作需要锁定当前线程，仅使用此线程
	runtime.LockOSThread()

	// veth peer 另外一端移到容器的 network namespace
	if err := netlink.LinkSetNsFd(*enLink, int(nsFD)); err != nil {
		log.Errorf("set link netns error: %v", err)
		return nil
	}

	// 记录当前的 network namespace，并在函数返回时切换回来
	origin, err := netns.Get()
	if err != nil {
		log.Errorf("get current netns error: %v", err)
		return nil
	}

	// 切换到容器的 network namespace
	if err := netns.Set(netns.NsHandle(nsFD)); err != nil {
		log.Errorf("set netns error: %v", err)
		return nil
	}

	return func() {
		// 切换回原有的 network namespace
		if err := netns.Set(origin); err != nil {
			log.Errorf("set netns error: %v", err)
		}
		origin.Close()
		runtime.UnlockOSThread()
		f.Close()
	}
}

// 将 ep 配置到 cinfo 指定的容器中
func configEndpointIpAddressAndRoute(ep *Endpoint, cinfo *container.ContainerInfo) error {
	// 获取 veth 另外一端的接口
	peerLink, err := netlink.LinkByName(ep.Device.PeerName)
	if err != nil {
		return fmt.Errorf("get peer link %s error: %v", ep.Device.PeerName, err)
	}

	// 将 ep 绑定到容器的 network namespace
	// 并切换到容器的 network namespace
	// 后续的配置将在容器的 network namespace 中进行
	defer enterContainerNetns(&peerLink, cinfo)()

	// 配置容器端 veth 接口的 IP 地址
	interfaceIP := *ep.Network.IpRange
	interfaceIP.IP = ep.IPAddress
	if err := setInterfaceIP(ep.Device.PeerName, interfaceIP.String()); err != nil {
		return fmt.Errorf("set interface %s ip %s error: %v", ep.Device.PeerName, interfaceIP.String(), err)
	}

	// 启动容器端 veth 接口
	if err := setInterfaceUP(ep.Device.PeerName); err != nil {
		return fmt.Errorf("set interface %s up error: %v", ep.Device.PeerName, err)
	}

	// 配置容器的默认路由
	_, cidr, _ := net.ParseCIDR("0.0.0.0/0")
	defaultRoute := &netlink.Route{
		LinkIndex: peerLink.Attrs().Index,
		Gw:        ep.Network.IpRange.IP,
		Dst:       cidr,
	}

	// 添加默认路由
	if err := netlink.RouteAdd(defaultRoute); err != nil {
		return fmt.Errorf("add default route %v error: %v", defaultRoute, err)
	}
	return nil
}

// 配置端口映射
// *:hostPort -> containerIP:containerPort
func configPortMapping(ep *Endpoint, cinfo *container.ContainerInfo) error {
	for _, pm := range cinfo.PortMapping {
		portMapping := strings.Split(pm, ":")
		if len(portMapping) != 2 {
			log.Errorf("port mapping format error, %v", pm)
		}
		iptablesCmd := fmt.Sprintf(
			"-t nat -A PREROUTING -p tcp -m tcp --dport %s -j DNAT --to-destination %s:%s",
			portMapping[0], ep.IPAddress.String(), portMapping[1])
		cmd := exec.Command("iptables", strings.Split(iptablesCmd, " ")...)
		output, err := cmd.Output()
		if err != nil {
			log.Errorf("iptables Output, %v", output)
			continue
		}
	}
	return nil
}

// 删除端口映射
func deletePortMapping(ep *Endpoint) error {
	for _, pm := range ep.PortMapping {
		portMapping := strings.Split(pm, ":")
		if len(portMapping) != 2 {
			log.Errorf("port mapping format error, %v", pm)
		}
		iptablesCmd := fmt.Sprintf(
			"-t nat -D PREROUTING -p tcp -m tcp --dport %s -j DNAT --to-destination %s:%s",
			portMapping[0], ep.IPAddress.String(), portMapping[1])
		cmd := exec.Command("iptables", strings.Split(iptablesCmd, " ")...)
		output, err := cmd.Output()
		if err != nil {
			log.Errorf("iptables Output, %v", output)
			continue
		}
	}
	return nil
}

func Connect(nwName string, cinfo *container.ContainerInfo) error {
	// 获取网络对象
	nw, ok := networks[nwName]
	if !ok {
		return fmt.Errorf("no such network %s", nwName)
	}

	// 分配 IP 地址
	ip, err := ipAllocator.Allocate(nw.IpRange)
	if err != nil {
		return err
	}

	// 创建端点对象
	ep := &Endpoint{
		ID:         fmt.Sprintf("%s-%s", cinfo.Id, nw.Name),
		IPAddress:  ip,
		MacAddress: nil,
		Network:    nw,
	}

	// 连接网络和端点
	if err := drivers[nw.Driver].Connect(nw, ep); err != nil {
		return err
	}

	// 配置容器内的网络接口和路由
	if err := configEndpointIpAddressAndRoute(ep, cinfo); err != nil {
		return err
	}

	return configPortMapping(ep, cinfo)
}

func Disconnect(nwName string, cinfo *container.ContainerInfo) error {
	return nil
}
