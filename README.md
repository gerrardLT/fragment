# Fragment - 大文件分片上传下载工具

这是一个使用Golang编写的工具，用于将大文件（如4GB）切分成多个分片（如10个400MB的分片），并使用0g-storage-client进行上传和下载。

## 功能特性

- ✅ 将大文件切分成指定大小的分片（默认400MB）
- ✅ 使用0g-storage-client上传分片到0G Storage网络
- ✅ 从0G Storage网络下载分片并合并还原原始文件
- ✅ 支持并发上传和下载
- ✅ 自动保存和加载分片的root hash映射

