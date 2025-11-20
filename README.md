# Fragment - 大文件分片上传下载工具

这是一个使用Golang编写的工具，用于将大文件（如4GB）切分成多个分片（如10个400MB的分片），并使用0g-storage-client进行上传和下载。

## 功能特性

- ✅ 将大文件切分成指定大小的分片（默认400MB）
- ✅ 使用0g-storage-client上传分片到0G Storage网络
- ✅ 从0G Storage网络下载分片并合并还原原始文件
- ✅ 支持并发上传和下载
- ✅ 自动保存和加载分片的root hash映射

## 前置要求

1. Go 1.21 或更高版本
2. 0g-storage-client Go SDK
3. 0G Storage网络访问权限（RPC端点和私钥）

## 安装依赖

```bash
go get github.com/0glabs/0g-storage-client@latest
```

## 配置

在运行程序前，需要修改 `main.go` 中的配置参数：

```go
config := Config{
    InputFile:    "largefile.dat", // 4GB文件路径
    OutputDir:    "./chunks",      // 分片存储目录
    EvmRpc:       "https://evmrpc-testnet.0g.ai",
    IndexerRpc:   "https://indexer-storage-testnet-turbo.0g.ai",
    PrivateKey:   "YOUR_PRIVATE_KEY", // 请替换为实际私钥（64字符，无0x前缀）
    FragmentSize: chunkSize,          // 400MB
}
```

### 重要配置说明

- **InputFile**: 要切分的大文件路径（如4GB文件）
- **OutputDir**: 分片文件的存储目录
- **EvmRpc**: 0G Storage网络的EVM RPC端点
- **IndexerRpc**: 0G Storage网络的Indexer RPC端点
- **PrivateKey**: 用于签名交易的私钥（64个十六进制字符，不包含0x前缀）
- **FragmentSize**: 每个分片的大小（默认400MB = 400 * 1024 * 1024 字节）

## 使用方法

### 1. 编译程序

```bash
go build -o fragment main.go
```

### 2. 运行程序

```bash
./fragment
```

或者直接运行：

```bash
go run main.go
```

### 3. 程序执行流程

程序会自动执行以下步骤：

1. **文件切分**: 将4GB文件切分成10个400MB的分片
   - 分片文件保存在 `OutputDir` 目录中
   - 文件命名格式：`chunk_000.dat`, `chunk_001.dat`, ...

2. **上传分片**: 使用0g-storage-client上传所有分片
   - 每个分片都会上传到0G Storage网络
   - 保存每个分片的root hash到 `hash_map.txt` 文件

3. **下载并合并**: 从0G Storage网络下载所有分片并合并
   - 根据保存的root hash下载每个分片
   - 按顺序合并所有分片还原原始文件
   - 输出文件：`原文件名.reconstructed`

## Fragment Size参数说明

在0g-storage-client中，fragment size参数用于控制文件上传时的分片大小。本程序通过以下方式处理：

1. **文件切分阶段**: 将大文件切分成400MB的分片，匹配fragment size
2. **上传阶段**: 每个400MB的分片作为一个独立文件上传，0g-storage-client会根据文件大小自动处理内部fragment

**注意**: 如果0g-storage-client的API支持直接设置fragment size参数，可以在创建Uploader时传入相应配置。

## 输出文件说明

- `chunks/chunk_000.dat` ~ `chunks/chunk_009.dat`: 切分后的分片文件
- `chunks/hash_map.txt`: 分片文件名和root hash的映射文件
- `largefile.dat.reconstructed`: 下载并合并后的还原文件

## 错误处理

程序包含完整的错误处理机制：

- 文件操作错误会立即返回并显示错误信息
- 上传/下载失败会记录错误并继续处理其他分片
- 所有错误都会输出详细的错误信息

## 注意事项

1. **私钥安全**: 请妥善保管私钥，不要将私钥提交到版本控制系统
2. **网络连接**: 确保能够访问0G Storage网络的RPC端点
3. **磁盘空间**: 确保有足够的磁盘空间存储分片文件和合并后的文件
4. **Root Hash**: 当前实现使用简化方法计算root hash，实际使用时应该使用0g-storage-client提供的API获取真正的Merkle tree root hash

## API适配说明

由于0g-storage-client的API可能因版本而异，以下部分可能需要根据实际API调整：

1. **Root Hash获取**: `calculateFileRootHash` 函数是简化实现，实际应该从`UploadFile`的返回结果中获取root hash
2. **Fragment Size设置**: 如果API支持，应该在创建Uploader时设置fragment size参数
3. **下载API**: 确保`Download`方法的参数格式与API一致

## 示例输出

```
步骤1: 开始切分文件...
文件总大小: 4294967296 字节 (4.00 GB)
  创建分片 0: ./chunks/chunk_000.dat (大小: 419430400 字节)
  创建分片 1: ./chunks/chunk_001.dat (大小: 419430400 字节)
  ...
文件切分完成，共生成 10 个分片

步骤2: 开始上传分片...
选择了 3 个节点
  上传分片 0: ./chunks/chunk_000.dat
  分片 0 上传成功，rootHash: 0x..., txHash: 0x...
  ...
上传完成，共上传 10 个分片

步骤3: 开始下载并合并分片...
  下载分片 0: ./chunks/chunk_000.dat (rootHash: 0x...)
  分片 0 下载并合并完成 (大小: 419430400 字节)
  ...
下载并合并完成，输出文件: largefile.dat.reconstructed
```

## 许可证

MIT License

## 贡献

欢迎提交Issue和Pull Request！
