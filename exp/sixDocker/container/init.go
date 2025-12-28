// exp/sixDocker/container/init.go

package container

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func RunContainerInitProcess() error {
	// 从父进程的第4个文件描述符中读取管道传递过来的命令
	cmdArray := readUserCommand()
	if len(cmdArray) == 0 {
		return fmt.Errorf("Run container get user command error, cmdArray is nil")
	}

	//setUpMount()

	// 从当前进程看到的环境变量中查找可执行文件的路径
	path, err := exec.LookPath(cmdArray[0])
	if err != nil {
		log.Errorf("Exec loop path error %v", err)
		return err
	}
	log.Infof("Find path %s", path)
	if err := syscall.Exec(path, cmdArray[0:], os.Environ()); err != nil {
		log.Errorf(err.Error())
	}
	return nil
}

func readUserCommand() []string {
	// 在NewParentProcess中将pipe的读端设置为3号文件描述符
	// 在init进程中通过3号文件描述符获取管道读端, 设置pipe的Name为pipe
	pipe := os.NewFile(uintptr(3), "pipe")
	// 等待父进程通过管道写入命令(阻塞) 也就是sendInitCommand函数的执行
	msg, err := ioutil.ReadAll(pipe)
	if err != nil {
		log.Errorf("init read pipe error %v", err)
		return nil
	}
	msgStr := string(msg)
	return strings.Split(msgStr, " ")
}
