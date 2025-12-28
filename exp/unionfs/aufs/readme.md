## 创建 aufs 文件系统

命令: ```sudo mount -t aufs -o dirs=./container-layer/:./image-layer4:./image-layer3:./image-layer2:./image-layer1 none ./mnt```

解释: 这条命令把多个目录叠加成一个联合文件系统挂载到 ./mnt：

* `./container-layer/` 是最上层，可写层，所有修改都会写到这里；
* `./image-layer4` ~ `./image-layer1` 是只读层，顺序从上到下依次叠加；
* AUFS 会按顺序查找文件，先找上层，找不到再找下层；
* `none` 是占位，AUFS 不需要实际设备；
* 操作 ./mnt 就像操作一个“容器文件系统”，修改只影响最上层。

核心概念: 层的顺序决定覆盖关系，最上层可写，下层只读。

特性: COW(COPY ON WRITE)

---