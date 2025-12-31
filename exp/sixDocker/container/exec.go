// exp/sixDocker/container/exec.go

package container

import (
	"fmt"
	"os"
	"os/exec"
	_ "sixDocker/nsenter"
	"strings"

	log "github.com/sirupsen/logrus"
)

const ENV_EXEC_PID = "sixDocker_pid"
const ENV_EXEC_CMD = "sixDocker_cmd"

func ExecContainer(containerName string, commandArray []string) error {
	// 获取容器PID
	containerInfo, err := GetContainerInfoByName(containerName)
	if err != nil {
		return err
	}
	pid := containerInfo.Pid
	if pid == "" {
		return fmt.Errorf("cannot find container %s pid", containerName)
	}

	// 拼接命令字符串
	cmdStr := strings.Join(commandArray, " ")
	log.Infof("container pid %s", pid)
	log.Infof("exec command %s", cmdStr)

	// 创建命令并将当前进程的标准输入输出错误重定向到子进程
	cmd := exec.Command("/proc/self/exe", "exec")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 传递环境变量
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%s", ENV_EXEC_PID, pid),
		fmt.Sprintf("%s=%s", ENV_EXEC_CMD, cmdStr))

	if err := cmd.Run(); err != nil {
		log.Errorf("Exec container %s command %s error: %v", containerName, cmdStr, err)
		return err
	}
	return nil
}
