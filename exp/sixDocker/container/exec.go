// exp/sixDocker/container/exec.go

package container

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	_ "sixDocker/nsenter"
	"strings"

	log "github.com/sirupsen/logrus"
)

const ENV_EXEC_PID = "sixDocker_pid"
const ENV_EXEC_CMD = "sixDocker_cmd"

func ExecContainer(containerId string, commandArray []string) error {
	// 获取容器PID
	pid, err := getContainerPidById(containerId)
	if err != nil {
		return err
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
		log.Errorf("Exec container %s command %s error: %v", containerId, cmdStr, err)
		return err
	}
	return nil
}

func getContainerPidById(containerId string) (string, error) {
	dirURL := fmt.Sprintf(DefaultInfoLocation, containerId)
	configFilePath := path.Join(dirURL, ConfigName)
	contentBytes, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("Read file %s error: %v", configFilePath, err)
		return "", err
	}
	var containerInfo ContainerInfo
	if err := json.Unmarshal(contentBytes, &containerInfo); err != nil {
		log.Errorf("Unmarshal container info error: %v", err)
		return "", err
	}
	return containerInfo.Pid, nil
}
