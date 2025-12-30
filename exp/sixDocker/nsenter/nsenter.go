// exp/sixDocker/nsenter/nsenter.go

//go:build linux && cgo
// +build linux,cgo

package nsenter

/*
#define _GNU_SOURCE
#include <errno.h>
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>

__attribute__((constructor)) void enter_namespace() {
    char *sixDocker_pid = getenv("sixDocker_pid");
    if (!sixDocker_pid) {
        return;
    }

    char *sixDocker_cmd = getenv("sixDocker_cmd");
    if (!sixDocker_cmd) {
        return;
    }

    char nspath[1024];
    char *namespaces[] = { "ipc", "uts", "net", "pid", "mnt" };

    for (int i = 0; i < 5; i++) {
		// 拼接命名空间路径
        snprintf(nspath, sizeof(nspath),
                 "/proc/%s/ns/%s", sixDocker_pid, namespaces[i]);
        // 打开命名空间文件
		int fd = open(nspath, O_RDONLY);
        if (fd == -1) {
            continue;
        }
		// 切换命名空间
        setns(fd, 0);
		// 关闭文件描述符，防止宿主机资源泄漏到容器内
        close(fd);
    }

    int ret = system(sixDocker_cmd);
    (void)ret;
    exit(0);
}
*/
import "C"
