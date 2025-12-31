// exp/sixDocker/container/container_process.go

package container

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/olekukonko/tablewriter"

	log "github.com/sirupsen/logrus"
)

var (
	RUNNING             string = "running"
	STOPPED             string = "stopped"
	EXIT                string = "exited"
	IMAGEDIR            string = "/var/run/sixDocker/images"
	READONLYLAYERDIR    string = "/var/run/sixDocker/readOnlyLayer"
	DefaultInfoLocation string = "/workspace/projects/go/dockerDev/run/containers/%s"
	ConfigName          string = "config.json"
	ContainerLogFile    string = "container.log"
)

type ContainerInfo struct {
	Pid         string   `json:"pid"`         // 容器init进程的pid
	Id          string   `json:"id"`          // 容器id
	Name        string   `json:"name"`        // 容器名称
	Command     string   `json:"command"`     // 容器内init进程要执行的命令
	CreatedTime string   `json:"createdTime"` // 容器创建时间
	Status      string   `json:"status"`      // 容器状态
	Volume      []string `json:"volume"`      // 容器挂载的卷
}

func NewParentProcess(tty bool) (*exec.Cmd, *os.File) {
	readPipe, writePipe, err := NewPipe()
	if err != nil {
		log.Errorf("New pipe error %v", err)
		return nil, nil
	}
	cmd := exec.Command("/proc/self/exe", "init")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
	}
	if tty {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	// UNIX会在子进程启动前会将ExtraFiles中的文件描述符从3开始依次往后分配，也就是说描述符是属于父进程
	// 启动子进程后 子进程会继承父进程的文件描述符表
	// 因此在init进程中 通过3号文件描述符就可以获取到管道的读端
	cmd.ExtraFiles = []*os.File{readPipe}
	return cmd, writePipe
}

func NewPipe() (*os.File, *os.File, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return read, write, nil
}

func NewWorkSpace(containerName string, ImageName string, volumes []string) (string, error) {
	log.Infof("Creating workspace for container %s", containerName)
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	ufsDir := path.Join(containerDir, "ufs")
	mntURL := path.Join(containerDir, "mnt")
	readOnlyLayerDir, err := CreateReadOnlyLayer(ImageName)
	if err != nil {
		return "", err
	}
	writeLayerDir, workLayerDir, err := CreateWriterLayer(ufsDir)
	if err != nil {
		return "", err
	}
	if err := CreateMountPoint(writeLayerDir, workLayerDir, readOnlyLayerDir, mntURL); err != nil {
		return "", err
	}
	for _, v := range volumes {
		parts := strings.Split(v, ":")

		if len(parts) < 2 {
			log.Errorf("Invalid volume format: %s", v)
			continue
		}

		source := parts[0]
		target := parts[1]

		// 默认 rw
		mode := "rw"
		if len(parts) == 3 {
			mode = parts[2]
		}

		CreateVolume(source, target, mntURL, mode)
	}
	return mntURL, nil
}

func CreateReadOnlyLayer(imageName string) (string, error) {
	targetDir := path.Join(READONLYLAYERDIR, imageName)

	// 检查该目录是否已存在
	if _, err := os.Stat(targetDir); err == nil {
		log.Infof("ReadOnlyLayer for %s already exists at %s", imageName, targetDir)
		return targetDir, nil
	}

	// 如果不存在，准备解压
	imageFilePath := path.Join(IMAGEDIR, imageName+".tar")

	// 检查文件是否存在
	if _, err := os.Stat(imageFilePath); os.IsNotExist(err) {
		log.Errorf("Image file %s does not exist. Please make sure the image is available.", imageFilePath)
		return "", fmt.Errorf("image file %s does not exist", imageFilePath)
	}

	// 创建目标目录
	if err := os.MkdirAll(targetDir, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", targetDir, err)
		return "", err
	}

	// 执行解压命令
	log.Infof("Extracting %s to %s", imageFilePath, targetDir)
	if err := exec.Command("tar", "-xvf", imageFilePath, "-C", targetDir).Run(); err != nil {
		log.Errorf("Tar extract %s error. %v", imageFilePath, err)
		// 如果解压失败，建议清理掉创建失败的目录，防止下次误判
		os.RemoveAll(targetDir)
		return "", err
	}

	return targetDir, nil
}

func CreateWriterLayer(ufsDir string) (string, string, error) {
	writeLayer := path.Join(ufsDir, "writeLayer")
	if err := os.MkdirAll(writeLayer, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", writeLayer, err)
		return "", "", err
	}
	workLayer := path.Join(ufsDir, "workLayer")
	if err := os.MkdirAll(workLayer, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", workLayer, err)
		return "", "", err
	}
	return writeLayer, workLayer, nil
}

func CreateMountPoint(writeLayerDir string, workLayerDir string, readOnlyLayer string, mntUrl string) error {
	log.Infof("Creating mount point at %s", mntUrl)
	log.Infof("ReadOnlyLayer: %s, WriterLayer: %s", readOnlyLayer, writeLayerDir)

	// 创建挂载点目录 (Merged 层)
	if err := os.MkdirAll(mntUrl, 0777); err != nil {
		log.Errorf("Mkdir mntUrl %s error. %v", mntUrl, err)
		return err
	}

	// 拼接 Overlay 挂载参数
	// lowerdir: 只读镜像层
	// upperdir: 可写层
	// workdir: 必要中间层
	params := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", readOnlyLayer, writeLayerDir, workLayerDir)

	// 4. 执行 mount 命令
	// mount -t overlay overlay -o lowerdir=...,upperdir=...,workdir=... mntUrl
	cmd := exec.Command("mount", "-t", "overlay", "overlay", "-o", params, mntUrl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Errorf("Mount overlay error: %v", err)
		return err
	}
	return nil
}

func DeleteWorkSpace(containerName string) error {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	rootUrl := path.Join(containerDir, "ufs")
	mntUrl := path.Join(containerDir, "mnt")

	// 读取容器信息 获取卷信息
	configFilePath := path.Join(containerDir, ConfigName)
	content, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("Read file %s error: %v", configFilePath, err)
		return err
	}
	var containerInfo ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.Errorf("Unmarshal container info error: %v", err)
		return err
	}
	volumes := containerInfo.Volume
	DeleteMountPoint(mntUrl, volumes)
	DeleteWriteLayer(rootUrl)
	return nil
}

func DeleteMountPoint(mntUrl string, volumes []string) {
	// 先卸载所有卷挂载点
	for _, v := range volumes {
		parts := strings.Split(v, ":")
		if len(parts) < 2 {
			continue
		}
		target := parts[1]
		DeleteMountPointOfVolume(mntUrl, target)
	}

	// 卸载主挂载点，如果失败则尝试 lazy unmount
	cmd := exec.Command("umount", mntUrl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Warnf("Umount %s error, trying lazy unmount: %v", mntUrl, err)
		// 尝试 lazy unmount
		cmd = exec.Command("umount", "-l", mntUrl)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Errorf("Lazy umount %s error: %v", mntUrl, err)
			return // 如果卸载失败，不删除目录
		}
	}

	// 等待一下确保卸载完成
	time.Sleep(100 * time.Millisecond)

	// 删除目录
	if err := os.RemoveAll(mntUrl); err != nil {
		log.Errorf("Remove dir %s error: %v", mntUrl, err)
	}
}

func DeleteWriteLayer(rootUrl string) {
	writerURL := path.Join(rootUrl, "writeLayer")
	if err := os.RemoveAll(writerURL); err != nil {
		log.Errorf("Remove dir %s error: %v", writerURL, err)
	}
}

func CreateVolume(source string, target string, mntUrl string, mode string) {
	// 宿主机目录不存在则创建
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			log.Warnf("Source volume %s does not exist. Creating it.", source)
			if err := os.MkdirAll(source, 0777); err != nil {
				log.Errorf("Mkdir volume source dir %s error. %v", source, err)
			}
		}
	}

	// 容器内目录不存在则创建
	containerURL := path.Join(mntUrl, target)
	if err := os.MkdirAll(containerURL, 0777); err != nil {
		log.Errorf("Mkdir volume dir %s error. %v", containerURL, err)
	}

	// 挂载宿主机目录到容器内目录
	cmd := exec.Command("mount", "--bind", source, containerURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("Mount volume %s to %s error: %v", source, containerURL, err)
		return
	}

	// 如果是只读模式，则重新以只读方式挂载一次
	if mode == "ro" {
		cmd = exec.Command("mount", "-o", "remount,ro,bind", source, containerURL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Errorf("Remount volume %s to %s error: %v", source, containerURL, err)
		}
	}
}

func DeleteMountPointOfVolume(mntUrl string, target string) {
	containerURL := path.Join(mntUrl, target)
	cmd := exec.Command("umount", containerURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Warnf("Umount volume dir %s error, trying lazy unmount: %v", containerURL, err)
		// 尝试 lazy unmount
		cmd = exec.Command("umount", "-l", containerURL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Errorf("Lazy umount volume dir %s error: %v", containerURL, err)
		}
	}
}

func CommitContainer(containerName string, imageName string) error {
	containerURL := fmt.Sprintf(DefaultInfoLocation, containerName)
	if !checkContainerExistsByName(containerName) {
		log.Errorf("Container name %s does not exist", containerName)
		return fmt.Errorf("container name %s does not exist", containerName)
	}
	mntURL := path.Join(containerURL, "mnt")
	imageURL := path.Join(IMAGEDIR, imageName+".tar")
	cmd := exec.Command("tar", "-cvf", imageURL, "-C", mntURL, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Commit container %s failed: %v, output: %s", containerName, err, string(output))
		return err
	}
	log.Infof("Commit container %s to image %s successfully", containerName, imageName)
	return nil
}

// 打印正在运行的容器信息
func ListContainers() {
	dirURL := fmt.Sprintf(DefaultInfoLocation, "")
	dirURL = dirURL[:len(dirURL)-1]
	files, err := ioutil.ReadDir(dirURL)
	if err != nil {
		log.Errorf("Read dir %s error: %v", dirURL, err)
		return
	}
	var containers []ContainerInfo
	for _, file := range files {
		containerDir := path.Join(dirURL, file.Name())
		configFilePath := path.Join(containerDir, ConfigName)
		content, err := ioutil.ReadFile(configFilePath)
		if err != nil {
			log.Errorf("Read file %s error: %v", configFilePath, err)
			continue
		}
		var containerInfo ContainerInfo
		if err := json.Unmarshal(content, &containerInfo); err != nil {
			log.Errorf("Unmarshal container info error: %v", err)
			continue
		}
		containers = append(containers, containerInfo)
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "PID", "Command", "CreatedTime", "Status"})

	for _, container := range containers {
		table.Append([]string{
			container.Id,
			container.Name,
			container.Pid,
			container.Command,
			container.CreatedTime,
			container.Status,
		})
	}
	table.Render()
}

func LogContainer(containerName string) {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	logFilePath := path.Join(containerDir, ContainerLogFile)
	content, err := ioutil.ReadFile(logFilePath)
	if err != nil {
		log.Errorf("Read log file %s error: %v", logFilePath, err)
		return
	}
	fmt.Fprint(os.Stdout, string(content))
}

func StopContainer(containerName string) error {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	configFilePath := path.Join(containerDir, ConfigName)

	content, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("Read file %s error: %v", configFilePath, err)
		return err
	}

	var containerInfo ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.Errorf("Unmarshal container info error: %v", err)
		return err
	}

	// 检查逻辑状态：如果已经是 STOPPED，直接返回
	if containerInfo.Status == STOPPED {
		log.Infof("Container %s has already been stopped, skip kill.", containerName)
		return nil
	}

	// 检查系统进程：使用 kill -0 探测进程是否存在
	// kill -0 不会发送信号，但会进行权限和进程存在性检查
	pidInt, _ := strconv.Atoi(containerInfo.Pid)
	if err := syscall.Kill(pidInt, 0); err != nil {
		log.Warnf("Process %d for container %s not found in system, skipping kill.", pidInt, containerName)
	} else {
		// 只有进程存在才执行真正的 kill
		log.Infof("Killing container %s (pid: %s)", containerName, containerInfo.Pid)
		cmd := exec.Command("kill", "-9", containerInfo.Pid)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Errorf("Kill container %s failed: %v, output: %s", containerName, err, string(output))
			return err
		}
	}

	// 更新容器状态并写回文件
	containerInfo.Status = STOPPED
	containerInfo.Pid = "" // 停止后清空 PID 也是一种常见的做法
	updatedContent, err := json.Marshal(containerInfo)
	if err != nil {
		log.Errorf("Marshal updated container info error: %v", err)
		return err
	}

	if err := os.WriteFile(configFilePath, updatedContent, 0622); err != nil {
		log.Errorf("Write updated container info to file %s error: %v", configFilePath, err)
		return err
	}

	return nil
}

func GetContainerInfoByName(containerName string) (*ContainerInfo, error) {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	if !checkContainerExistsByName(containerName) {
		return nil, fmt.Errorf("container name %s does not exist", containerName)
	}
	configFilePath := path.Join(containerDir, ConfigName)
	content, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("Read file %s error: %v", configFilePath, err)
		return nil, err
	}
	var containerInfo ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.Errorf("Unmarshal container info error: %v", err)
		return nil, err
	}
	return &containerInfo, nil
}

func CreateContainerInfoByName(containerName string, pid int, commandArray []string, volume []string) (*ContainerInfo, error) {
	containerId := randStringBytes(10)
	if containerName == "" {
		containerName = containerId
	}
	log.Infof("Creating container info for %s", containerName)
	if checkContainerExistsByName(containerName) {
		return nil, fmt.Errorf("container name %s already exists", containerName)
	}
	command := strings.Join(commandArray, " ")
	CreatedTime := time.Now().Format("2006-01-02 15:04:05")
	containerInfo := &ContainerInfo{
		Name:        containerName,
		Pid:         strconv.Itoa(pid),
		Id:          containerId,
		Command:     command,
		CreatedTime: CreatedTime,
		Status:      RUNNING,
		Volume:      volume,
	}
	jsonBytes, err := json.Marshal(containerInfo)
	if err != nil {
		log.Errorf("Record container info error: %v", err)
		return nil, err
	}
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	if err := os.MkdirAll(containerDir, 0622); err != nil {
		log.Errorf("MkdirAll %s error: %v", containerDir, err)
		return nil, err
	}
	fileName := path.Join(containerDir, ConfigName)
	if err := os.WriteFile(fileName, jsonBytes, 0622); err != nil {
		log.Errorf("Write file %s error: %v", fileName, err)
		return nil, err
	}
	return containerInfo, nil
}

func UpdateContainerInfoByName(containerName string, update *ContainerInfo) error {
	if !checkContainerExistsByName(containerName) {
		return fmt.Errorf("container %s does not exist", containerName)
	}

	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	configFilePath := path.Join(containerDir, ConfigName)

	// 读取旧配置
	content, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return err
	}

	var old ContainerInfo
	if err := json.Unmarshal(content, &old); err != nil {
		return err
	}

	// 字段更新
	if update.Id != old.Id || update.Name != old.Name || update.CreatedTime != old.CreatedTime {
		return fmt.Errorf("container ID, Name, and CreatedTime cannot be updated")
	}
	if update.Pid != "" {
		old.Pid = update.Pid
	}
	if update.Command != "" {
		old.Command = update.Command
	}
	if update.Status != "" {
		old.Status = update.Status
	}
	if update.Volume != nil {
		old.Volume = update.Volume
	}

	// 写回文件
	newContent, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(configFilePath, newContent, 0644)
}

func deleteContainerInfo(containerName string) error {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	if err := os.RemoveAll(containerDir); err != nil {
		log.Errorf("Remove file %s error: %v", containerDir, err)
		return err
	}
	return nil
}

func checkContainerExistsByName(containerName string) bool {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerName)
	if _, err := os.Stat(containerDir); err == nil {
		return true
	}
	return false
}

func randStringBytes(n int) string {
	const letterBytes = "0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func DeleteContainer(containerName string, force_delete bool) error {
	// 停止容器
	if force_delete {
		if err := StopContainer(containerName); err != nil {
			log.Errorf("Stop container error: %v", err)
			return err
		}
	}
	// 检查容器是否停止
	containerInfo, err := GetContainerInfoByName(containerName)
	if err != nil {
		log.Errorf("Get container info error: %v", err)
		return err
	}
	if containerInfo.Status == RUNNING {
		return fmt.Errorf("cannot remove a running container, please stop it first")
	}
	// 卸载挂载点 & 删除容器文件系统
	if err := DeleteWorkSpace(containerName); err != nil {
		log.Errorf("Delete workspace error: %v", err)
		return err
	}
	// 删除容器信息
	if err := deleteContainerInfo(containerName); err != nil {
		log.Errorf("Delete container info error: %v", err)
		return err
	}
	return nil
}

// ShowAllImages 列出 IMAGEDIR 目录下所有的镜像名称及其创建时间
func ShowAllImages() error {
	// 读取镜像存储目录
	files, err := os.ReadDir(IMAGEDIR)
	if err != nil {
		log.Errorf("Read dir %s error: %v", IMAGEDIR, err)
		return err
	}

	// 定义一个结构体来存储镜像信息
	type imageInfo struct {
		name    string
		created string
	}

	var images []imageInfo
	for _, file := range files {
		// 仅处理文件，且后缀为 .tar
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".tar") {
			// 获取文件信息（包含时间戳）
			info, err := file.Info()
			if err != nil {
				continue // 如果获取不到信息则跳过
			}

			imageName := strings.TrimSuffix(file.Name(), ".tar")
			// 格式化时间为：2023-10-27 10:00:00
			createdTime := info.ModTime().Format("2006-01-02 15:04:05")

			images = append(images, imageInfo{
				name:    imageName,
				created: createdTime,
			})
		}
	}

	// 检查镜像列表是否为空
	if len(images) == 0 {
		fmt.Printf("No images found in %s\n", IMAGEDIR)
		return nil
	}

	// 初始化表格渲染器
	table := tablewriter.NewWriter(os.Stdout)
	// 修改表头，增加 CREATED
	table.SetHeader([]string{"IMAGE NAME", "CREATED"})

	// --- 样式配置 ---
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetAutoWrapText(false)
	table.SetTablePadding("\t")

	// 填充数据
	for _, img := range images {
		table.Append([]string{img.name, img.created})
	}

	// 渲染输出
	table.Render()

	return nil
}
