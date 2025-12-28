// exp/sixDocker/cgroups/subsystems/subsystem.go

package subsystems

// 结构体放字段
type ResourceConfig struct {
	MemoryLimit string
	CpuShare    string
	CpuSet      string
}

// 接口放函数签名
type Subsystem interface {
	// 返回 子系统名称
	Name() string
	// 设置子系统中 cgroup 资源限制
	Set(path string, res *ResourceConfig) error
	// 将进程加入到子系统中指定的 cgroup
	Apply(path string, pid int) error
	// 移除子系统中指定的 cgroup
	Remove(path string) error
}

var (
	// SubsystemsIns 实例列表
	SubsystemsIns = []Subsystem{
		// 结构体实例指针 type=Subsystem
		&CpuSubSystem{},
		&CpusetSubSystem{},
		&MemorySubSystem{},
	}
)
