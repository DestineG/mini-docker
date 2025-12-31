// exp/sixDocker/container/exec.go

package container

import (
	"fmt"
	"io/ioutil"
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

	// 获取容器的环境变量
	env_path := fmt.Sprintf("/proc/%s/environ", pid)
	contentBytes, err := ioutil.ReadFile(env_path)
	if err != nil {
		log.Errorf("Read container %s env file %s error: %v", containerName, env_path, err)
		return err
	}
	envSlice := strings.Split(string(contentBytes), "\u0000")

	// 预分配空间提高效率
	finalEnv := append(os.Environ(),
		fmt.Sprintf("%s=%s", ENV_EXEC_PID, pid),
		fmt.Sprintf("%s=%s", ENV_EXEC_CMD, cmdStr),
	)

	for _, env := range envSlice {
		// 过滤掉空字符串或无效格式，防止注入失败
		if env != "" && strings.Contains(env, "=") {
			finalEnv = append(finalEnv, env)
		}
	}

	// 将宿主机和容器的环境变量传递给子进程
	cmd.Env = finalEnv

	if err := cmd.Run(); err != nil {
		log.Errorf("Exec container %s command %s error: %v", containerName, cmdStr, err)
		return err
	}
	return nil
}
