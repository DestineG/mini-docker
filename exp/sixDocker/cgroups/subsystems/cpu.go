// exp/sixDocker/cgroups/subsystems/cpu.go

package subsystems

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
)

type CpuSubSystem struct{}

func (s *CpuSubSystem) Name() string {
	return "cpu"
}

// 设置 cpu 子系统中 cgroup 资源限制
func (s *CpuSubSystem) Set(cgroupPath string, res *ResourceConfig) error {
	// subsysCgroupPath=/sys/fs/cgroup/cpu,cpuacct + cgroupPath
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, true); err == nil {
		// 如果ResourceConfig中配置了 CpuShare 则允许设置 cpu.shares 文件
		if res.CpuShare != "" {
			// ioutil.WriteFile 要求写入的内容是字节切片(覆盖写入)
			// []byte(string) 将字符串转换为字节切片
			// 0644 文件权限
			if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "cpu.shares"), []byte(res.CpuShare), 0644); err != nil {
				return fmt.Errorf("set cgroup cpu.shares fail %v", err)
			}
		}
		return nil
	} else {
		return err
	}
}

// 将进程加入到 cpu 子系统中指定的 cgroup; 也就是将pid写入到 cgroup 的 tasks 文件中
func (s *CpuSubSystem) Apply(cgroupPath string, pid int) error {
	// 获取 cgroup 配置目录
	// subsysCgroupPath=/sys/fs/cgroup/cpu,cpuacct + cgroupPath
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, true); err == nil {
		// ioutil.WriteFile 要求写入的内容是字节切片(覆盖写入)
		// strconv.Itoa(pid) 将整数 pid 转换为字符串; []byte(string) 将字符串转换为字节切片
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
			return fmt.Errorf("set cgroup cpu tasks fail %v", err)
		}
		return nil
	} else {
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}

// 移除 cpu 子系统中指定的 cgroup
func (s *CpuSubSystem) Remove(cgroupPath string) error {
	// 获取 cgroup 配置目录
	// subsysCgroupPath=/sys/fs/cgroup/cpu,cpuacct + cgroupPath
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, false); err == nil {
		// 删除该 cgroup 目录
		return os.RemoveAll(subsysCgroupPath)
	} else {
		return err
	}
}
