// exp/sixDocker/newwork/bridge.go

package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type BridgeNetworkDriver struct{}

func (b *BridgeNetworkDriver) Name() string {
	return "bridge"
}

func (b *BridgeNetworkDriver) Create(subnet string, name string) (*Network, error) {
	// 解析 cidr 子网：*.*.*.*/n -> ip, ipRange
	ip, ipRange, _ := net.ParseCIDR(subnet)
	ipRange.IP = ip

	// 创建 Network 对象
	nw := &Network{
		Name:    name,
		IpRange: ipRange,
		Driver:  b.Name(),
	}

	// 初始化 bridge 设备
	err := b.initBridge(nw)
	if err != nil {
		return nil, err
	}
	return nw, nil
}

func (b *BridgeNetworkDriver) Delete(nw *Network) error {
	nwName := nw.Name
	iface, err := netlink.LinkByName(nwName)
	if err != nil {
		return err
	}
	if err := teardownIPTables(nwName, nw.IpRange); err != nil {
		return err
	}
	return netlink.LinkDel(iface)
}

func (b *BridgeNetworkDriver) Connect(nw *Network, endpoint *Endpoint) error {
	// 获取网桥
	nwName := nw.Name
	iface, err := netlink.LinkByName(nwName)
	if err != nil {
		return err
	}

	// 创建 veth 的属性，并指定其所属桥（master = bridge）
	la := netlink.NewLinkAttrs()
	la.Name = endpoint.ID[:5]
	la.MasterIndex = iface.Attrs().Index

	// 创建 veth 对象
	endpoint.Device = netlink.Veth{
		LinkAttrs: la,
		PeerName:  "cif-" + endpoint.ID[:5],
	}

	// 添加 veth 到系统
	if err := netlink.LinkAdd(&endpoint.Device); err != nil {
		return fmt.Errorf("Error add veth pair for endpoint %s: %v", endpoint.ID, err)
	}

	// 启动 veth 设备
	if err := netlink.LinkSetUp(&endpoint.Device); err != nil {
		return fmt.Errorf("Error set veth up for endpoint %s: %v", endpoint.ID, err)
	}
	return nil
}

func (b *BridgeNetworkDriver) Disconnect(nw *Network, endpoint *Endpoint) error {
	// 获取 veth 接口
	vethName := endpoint.Device.Attrs().Name
	iface, err := netlink.LinkByName(vethName)
	if err != nil {
		if _, ok := err.(netlink.LinkNotFoundError); ok {
			// 接口已经不存在，直接返回
			return nil
		}
		return fmt.Errorf("failed to get veth %s: %v", vethName, err)
	}

	// 删除 veth 接口
	if err := netlink.LinkDel(iface); err != nil {
		return fmt.Errorf("failed to delete veth %s: %v", vethName, err)
	}

	return nil
}

func (b *BridgeNetworkDriver) initBridge(nw *Network) error {
	nwName := nw.Name

	// 创建网桥设备
	if err := createBridgeInterface(nwName); err != nil {
		return fmt.Errorf("Error add bridge %s Error: %v", nwName, err)
	}

	// 配置网桥 ip 地址
	gatewayIP := *nw.IpRange
	gatewayIP.IP = nw.IpRange.IP
	if err := setInterfaceIP(nwName, gatewayIP.String()); err != nil {
		return fmt.Errorf("Error assigning address: %s on bridge: %s with an error of: %v", gatewayIP, nwName, err)
	}

	// 启动网桥设备
	if err := setInterfaceUP(nwName); err != nil {
		return fmt.Errorf("Error set bridge %s up with an error of: %v", nwName, err)
	}

	// 配置 Linux 防火墙/路由规则
	if err := setupIPTables(nwName, nw.IpRange); err != nil {
		return fmt.Errorf("Error setup iptables on bridge %s with an error of: %v", nwName, err)
	}
	return nil
}

func createBridgeInterface(nwName string) error {
	// 检查网桥是否已经存在
	_, err := net.InterfaceByName(nwName)
	if err == nil || !strings.Contains(err.Error(), "no such network interface") {
		return fmt.Errorf("bridge %s already exists", nwName)
	}

	// 创建网桥属性
	la := netlink.NewLinkAttrs()
	la.Name = nwName

	// 创建网桥
	br := &netlink.Bridge{LinkAttrs: la}

	// 添加网桥到系统
	if err := netlink.LinkAdd(br); err != nil {
		return fmt.Errorf("Bridge creation failed for bridge %s: %v", nwName, err)
	}
	return nil
}

func setInterfaceIP(nwName string, rawIP string) error {
	// 获取网桥
	retries := 2
	var iface netlink.Link
	var err error
	for i := 0; i < retries; i++ {
		iface, err = netlink.LinkByName(nwName)
		if err == nil {
			break
		}
		log.Debugf("Retrying to get interface %s: %v", nwName, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return err
	}

	// 解析 ip 地址
	ipNet, err := netlink.ParseIPNet(rawIP)
	if err != nil {
		return fmt.Errorf("Parsing IP address %s failed: %v", rawIP, err)
	}

	// 构建地址对象
	addr := &netlink.Addr{IPNet: ipNet}

	// 给网桥分配 ip 地址
	return netlink.AddrAdd(iface, addr)
}

func setInterfaceUP(nwName string) error {
	// 获取网桥
	iface, err := netlink.LinkByName(nwName)
	if err != nil {
		return fmt.Errorf("Failed to get interface %s: %v", nwName, err)
	}

	// 启动网桥
	if err := netlink.LinkSetUp(iface); err != nil {
		return fmt.Errorf("Failed to set interface %s up: %v", nwName, err)
	}
	return nil
}

// 给网桥配置 iptables 规则，实现 NAT 功能，可以访问外网
func setupIPTables(nwName string, ipRange *net.IPNet) error {
	// 构造规则参数：源地址是容器网段，且出口不是网桥自身的包，进行伪装
	args := []string{"-t", "nat", "-A", "POSTROUTING", "-s", ipRange.String(), "!", "-o", nwName, "-j", "MASQUERADE"}

	// 检查规则是否已经存在，避免重复添加
	// 将 -A 替换为 -C 进行检查
	checkArgs := make([]string, len(args))
	copy(checkArgs, args)
	checkArgs[2] = "-C"

	if err := exec.Command("iptables", checkArgs...).Run(); err == nil {
		log.Infof("iptables rule for %s already exists, skip adding", nwName)
		return nil
	}

	// 规则不存在，执行添加
	cmd := exec.Command("iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to setup iptables: %v, output: %s", err, string(output))
	}

	log.Infof("Successfully added iptables NAT rule for network %s", nwName)
	return nil
}

// teardownIPTables 在删除网桥时，同步清理掉之前创建的 NAT 规则
func teardownIPTables(nwName string, ipRange *net.IPNet) error {
	// 构造删除参数：将 -A 改为 -D
	args := []string{"-t", "nat", "-D", "POSTROUTING", "-s", ipRange.String(), "!", "-o", nwName, "-j", "MASQUERADE"}

	// 检查规则是否存在，如果不存在则不需要删除，直接返回
	checkArgs := make([]string, len(args))
	copy(checkArgs, args)
	checkArgs[2] = "-C"

	if err := exec.Command("iptables", checkArgs...).Run(); err != nil {
		log.Warnf("iptables rule for %s does not exist, no need to delete", nwName)
		return nil
	}

	// 规则存在，执行删除
	cmd := exec.Command("iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete iptables: %v, output: %s", err, string(output))
	}

	log.Infof("Successfully removed iptables NAT rule for network %s", nwName)
	return nil
}
