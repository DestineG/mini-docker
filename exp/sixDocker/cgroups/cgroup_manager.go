// exp/sixDocker/cgroups/cgroup_manager.go

package cgroups

import (
	"sixDocker/cgroups/subsystems"

	"github.com/sirupsen/logrus"
)

type CgroupManager struct {
	Path     string
	Resource *subsystems.ResourceConfig
}

// 创建CgroupManager实例
func NewCgroupManager(path string) *CgroupManager {
	return &CgroupManager{
		Path: path,
	}
}

// 在三个控制器中设置分别创建一个cgroup
func (cgroupMgr *CgroupManager) Set(res *subsystems.ResourceConfig) error {
	for _, subSysIns := range subsystems.SubsystemsIns {
		subSysIns.Set(cgroupMgr.Path, res)
	}
	return nil
}

// 将进程加入三个控制器中创建的cgroup
func (cgroupMgr *CgroupManager) Apply(pid int) error {
	for _, subSysIns := range subsystems.SubsystemsIns {
		subSysIns.Apply(cgroupMgr.Path, pid)
	}
	return nil
}

// 删除三个控制器中创建的cgroup
func (cgroupMgr *CgroupManager) Destroy() error {
	for _, subSysIns := range subsystems.SubsystemsIns {
		if err := subSysIns.Remove(cgroupMgr.Path); err != nil {
			logrus.Errorf("cgroup destroy err %v", err)
		}
	}
	return nil
}
