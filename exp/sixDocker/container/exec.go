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
	// 获取容器信息，拿到 PID
	containerInfo, err := GetContainerInfoByName(containerName)
	if err != nil {
		return fmt.Errorf("Get container %s info error: %v", containerName, err)
	}
	pid := containerInfo.Pid
	if pid == "" {
		return fmt.Errorf("cannot find container %s pid", containerName)
	}

	// 处理命令数组：为了防止 system() 解析时丢失引号，
	// 我们对每个参数进行单引号包裹，并转义原有的单引号。
	// 最终拼接成一个符合 Shell 转义规则的字符串。
	var escapedParts []string
	for _, arg := range commandArray {
		// 将 ' 替换为 '\'' (结束前一个引用，转义单引号本身，重新开始引用)
		escapedArg := "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		escapedParts = append(escapedParts, escapedArg)
	}
	cmdStr := strings.Join(escapedParts, " ")

	log.Infof("Container PID: %s", pid)
	log.Infof("Exec Command: %s", cmdStr)

	// 构造子进程。由于 nsenter.go 里的 C 语言 constructor 会拦截 "/proc/self/exe exec"
	// 所以我们再次启动自己，并进入拦截逻辑。
	cmd := exec.Command("/proc/self/exe", "exec")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 读取容器的环境变量，确保 exec 进去后能拿到容器内的 PATH 等配置
	envPath := fmt.Sprintf("/proc/%s/environ", pid)
	contentBytes, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("Read container %s env file %s error: %v", containerName, envPath, err)
	}
	// 容器内的 /proc/pid/environ 是以 \u0000 (null byte) 分隔的
	containerEnvs := strings.Split(string(contentBytes), "\u0000")

	// 准备最终的环境变量
	// 首先继承当前宿主机的环境变量（为了让子进程能运行）
	finalEnv := os.Environ()

	// 注入 C 语言层拦截需要的关键变量
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", ENV_EXEC_PID, pid))
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", ENV_EXEC_CMD, cmdStr))

	// 注入容器原有的环境变量
	for _, env := range containerEnvs {
		if env != "" && strings.Contains(env, "=") {
			// 这里可以根据需要决定是否覆盖宿主机的变量（如 PATH）
			finalEnv = append(finalEnv, env)
		}
	}

	cmd.Env = finalEnv

	// 运行子进程。C 语言层的 constructor 会在此处 setns 并执行命令后 exit(0)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Exec container %s error: %v", containerName, err)
	}
	return nil
}
