// exp/sixDocker/network/ipam.go

package network

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path"
	"strings"
)

const ipamDefaultAllocatorPath = "/var/run/sixDocker/network/ipam/subnet.json"

type IPAM struct {
	SubnetAllocatorPath string
	Subnets             map[string]string
}

var ipAllocator = &IPAM{
	SubnetAllocatorPath: ipamDefaultAllocatorPath,
}

func (ipam *IPAM) load() error {
	if _, err := os.Stat(ipam.SubnetAllocatorPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	subnetConfigFile, err := os.Open(ipam.SubnetAllocatorPath)
	if err != nil {
		return err
	}
	defer subnetConfigFile.Close()

	if ipam.Subnets == nil {
		ipam.Subnets = make(map[string]string)
	}

	decoder := json.NewDecoder(subnetConfigFile)
	err = decoder.Decode(&ipam.Subnets)
	if err != nil {
		log.Printf("Error load subnet file %s: %v", ipam.SubnetAllocatorPath, err)
		return err
	}
	return nil
}

func (ipam *IPAM) dump() error {
	ipamConfigFileDir, _ := path.Split(ipam.SubnetAllocatorPath)
	if _, err := os.Stat(ipamConfigFileDir); err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(ipamConfigFileDir, 0755)
		} else {
			return err
		}
	}

	subnetConfigFile, err := os.OpenFile(ipam.SubnetAllocatorPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer subnetConfigFile.Close()

	ipamConfigJson, err := json.Marshal(ipam.Subnets)
	if err != nil {
		return err
	}

	_, err = subnetConfigFile.Write(ipamConfigJson)
	if err != nil {
		return err
	}
	return nil
}

func (ipam *IPAM) Allocate(subnet *net.IPNet) (ip net.IP, err error) {
	ipam.Subnets = make(map[string]string)

	// 加载已有的分配信息
	if err := ipam.load(); err != nil {
		return nil, err
	}

	_, subnet, _ = net.ParseCIDR(subnet.String())
	one, size := subnet.Mask.Size()

	if _, exist := ipam.Subnets[subnet.String()]; !exist {
		ipam.Subnets[subnet.String()] = strings.Repeat("0", 1<<uint8(size-one))
	}

	// 标记分配第一个可用 IP 地址
	for c := range ipam.Subnets[subnet.String()] {
		if ipam.Subnets[subnet.String()][c] == '0' {
			ipalloc := []rune(ipam.Subnets[subnet.String()])
			ipalloc[c] = '1'
			ipam.Subnets[subnet.String()] = string(ipalloc)

			// 计算分配的 IP 地址
			ip = make(net.IP, len(subnet.IP))
			copy(ip, subnet.IP)

			for t := uint(4); t > 0; t-- {
				ip[4-t] += uint8(c >> ((t - 1) * 8))
			}
			ip[3] += 1
			break
		}
	}

	// 保存分配信息到磁盘
	if err := ipam.dump(); err != nil {
		return nil, err
	}
	return
}

func (ipam *IPAM) Release(subnet *net.IPNet, ip *net.IP) error {
	// 加载已有的分配信息
	ipam.Subnets = make(map[string]string)
	if err := ipam.load(); err != nil {
		return err
	}

	_, subnet, _ = net.ParseCIDR(subnet.String())

	c := 0
	releaseIP := ip.To4()
	releaseIP[3] -= 1
	for t := uint(4); t > 0; t-- {
		c += int(releaseIP[4-t]-subnet.IP[4-t]) << ((t - 1) * 8)
	}

	// 标记释放对应的 IP 地址
	ipalloc := []byte(ipam.Subnets[subnet.String()])
	ipalloc[c] = '0'
	ipam.Subnets[subnet.String()] = string(ipalloc)

	// 保存分配信息到磁盘
	if err := ipam.dump(); err != nil {
		return err
	}
	return nil
}
