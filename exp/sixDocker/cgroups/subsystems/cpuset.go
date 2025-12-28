// exp/sixDocker/cgroups/subsystems/cpuset.go

package subsystems

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
)

type CpusetSubSystem struct{}

func (s *CpusetSubSystem) Name() string {
	return "cpuset"
}

func (s *CpusetSubSystem) Set(cgroupPath string, res *ResourceConfig) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, true); err == nil {
		// 如果ResourceConfig中配置了 CpuSet 则允许设置 cpuset.cpus 文件
		if res.CpuSet != "" {
			// ioutil.WriteFile 要求写入的内容是字节切片(覆盖写入)
			// []byte(string) 将字符串转换为字节切片
			if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "cpuset.cpus"), []byte(res.CpuSet), 0644); err != nil {
				return fmt.Errorf("set cgroup cpuset.cpus fail %v", err)
			}
		}
		return nil
	} else {
		return err
	}
}

func (s *CpusetSubSystem) Apply(cgroupPath string, pid int) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, true); err == nil {
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
			return fmt.Errorf("set cgroup cpuset tasks fail %v", err)
		}
		return nil
	} else {
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}

func (s *CpusetSubSystem) Remove(cgroupPath string) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, false); err == nil {
		return os.RemoveAll(subsysCgroupPath)
	} else {
		return err
	}
}
