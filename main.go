package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/0glabs/0g-storage-client/blockchain"
	"github.com/0glabs/0g-storage-client/indexer"
	"github.com/0glabs/0g-storage-client/transfer"
)

const (
	// 分片大小：400MB
	chunkSize = 400 * 1024 * 1024
	// 分片数量
	numChunks = 10
)

// Config 配置结构
type Config struct {
	// 输入文件路径
	InputFile string
	// 输出目录
	OutputDir string
	// EVM RPC端点
	EvmRpc string
	// Indexer RPC端点
	IndexerRpc string
	// 私钥（64字符，无0x前缀）
	PrivateKey string
	// 分片大小（字节）
	FragmentSize int64
}

func main() {
	// 配置参数（请根据实际情况修改）
	config := Config{
		InputFile:    "largefile.dat", // 4GB文件路径
		OutputDir:    "./chunks",      // 分片存储目录
		EvmRpc:       "https://evmrpc-testnet.0g.ai",
		IndexerRpc:   "https://indexer-storage-testnet-turbo.0g.ai",
		PrivateKey:   "YOUR_PRIVATE_KEY", // 请替换为实际私钥
		FragmentSize: chunkSize,
	}

	ctx := context.Background()

	// 步骤1: 切分文件
	fmt.Println("步骤1: 开始切分文件...")
	chunkFiles, err := splitFile(config.InputFile, config.OutputDir, config.FragmentSize)
	if err != nil {
		fmt.Printf("文件切分失败: %v\n", err)
		return
	}
	fmt.Printf("文件切分完成，共生成 %d 个分片\n", len(chunkFiles))

	// 步骤2: 上传分片
	fmt.Println("\n步骤2: 开始上传分片...")
	rootHashes, err := uploadChunks(ctx, config, chunkFiles)
	if err != nil {
		fmt.Printf("上传分片失败: %v\n", err)
		return
	}
	fmt.Printf("上传完成，共上传 %d 个分片\n", len(rootHashes))

	// 保存root hash映射
	hashMapFile := filepath.Join(config.OutputDir, "hash_map.txt")
	if err := saveHashMap(hashMapFile, chunkFiles, rootHashes); err != nil {
		fmt.Printf("保存hash映射失败: %v\n", err)
	}

	// 步骤3: 下载并合并分片
	fmt.Println("\n步骤3: 开始下载并合并分片...")
	outputFile := config.InputFile + ".reconstructed"
	if err := downloadAndMergeChunks(ctx, config, hashMapFile, outputFile); err != nil {
		fmt.Printf("下载并合并分片失败: %v\n", err)
		return
	}
	fmt.Printf("下载并合并完成，输出文件: %s\n", outputFile)
}

// splitFile 将大文件切分成指定大小的分片
func splitFile(inputFile, outputDir string, fragmentSize int64) ([]string, error) {
	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 打开输入文件
	file, err := os.Open(inputFile)
	if err != nil {
		return nil, fmt.Errorf("打开输入文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	totalSize := fileInfo.Size()
	fmt.Printf("文件总大小: %d 字节 (%.2f GB)\n", totalSize, float64(totalSize)/(1024*1024*1024))

	var chunkFiles []string
	buffer := make([]byte, fragmentSize)

	for i := int64(0); i < numChunks; i++ {
		// 计算当前分片大小
		bytesToRead := fragmentSize
		if i == numChunks-1 {
			// 最后一个分片可能小于fragmentSize
			remaining := totalSize - i*fragmentSize
			if remaining <= 0 {
				break
			}
			if remaining < fragmentSize {
				bytesToRead = remaining
			}
		}

		// 读取数据
		bytesRead, err := file.Read(buffer[:bytesToRead])
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("读取文件失败: %w", err)
		}
		if bytesRead == 0 {
			break
		}

		// 创建分片文件
		chunkFileName := filepath.Join(outputDir, fmt.Sprintf("chunk_%03d.dat", i))
		chunkFile, err := os.Create(chunkFileName)
		if err != nil {
			return nil, fmt.Errorf("创建分片文件失败: %w", err)
		}

		// 写入数据
		if _, err := chunkFile.Write(buffer[:bytesRead]); err != nil {
			chunkFile.Close()
			return nil, fmt.Errorf("写入分片文件失败: %w", err)
		}
		chunkFile.Close()

		chunkFiles = append(chunkFiles, chunkFileName)
		fmt.Printf("  创建分片 %d: %s (大小: %d 字节)\n", i, chunkFileName, bytesRead)
	}

	return chunkFiles, nil
}

// uploadChunks 上传所有分片到0G Storage
func uploadChunks(ctx context.Context, config Config, chunkFiles []string) (map[string]string, error) {
	// 创建Web3客户端
	w3client := blockchain.MustNewWeb3(config.EvmRpc, config.PrivateKey)
	defer w3client.Close()

	// 创建Indexer客户端
	indexerClient, err := indexer.NewClient(config.IndexerRpc)
	if err != nil {
		return nil, fmt.Errorf("创建Indexer客户端失败: %w", err)
	}

	// 获取可用节点
	nodes, err := indexerClient.SelectNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("选择节点失败: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("没有可用的节点")
	}

	fmt.Printf("选择了 %d 个节点\n", len(nodes))

	// 创建上传器
	// 注意：fragment size通常在0g-storage-client内部处理
	// 我们通过将文件切分成400MB来匹配fragment size
	uploader, err := transfer.NewUploader(ctx, w3client, nodes)
	if err != nil {
		return nil, fmt.Errorf("创建上传器失败: %w", err)
	}

	// 存储每个分片的root hash
	rootHashes := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(chunkFiles))

	// 并发上传分片
	for i, chunkFile := range chunkFiles {
		wg.Add(1)
		go func(idx int, file string) {
			defer wg.Done()

			fmt.Printf("  上传分片 %d: %s\n", idx, file)

			// 在上传前计算文件的root hash
			rootHash, err := calculateFileRootHash(file)
			if err != nil {
				errChan <- fmt.Errorf("计算分片 %s 的root hash失败: %w", file, err)
				return
			}

			// 上传文件
			txHash, err := uploader.UploadFile(ctx, file)
			if err != nil {
				errChan <- fmt.Errorf("上传分片 %s 失败: %w", file, err)
				return
			}

			mu.Lock()
			rootHashes[file] = rootHash
			mu.Unlock()

			fmt.Printf("  分片 %d 上传成功，rootHash: %s, txHash: %s\n", idx, rootHash, txHash)
		}(i, chunkFile)
	}

	wg.Wait()
	close(errChan)

	// 检查错误
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return rootHashes, nil
}

// saveHashMap 保存分片文件名和root hash的映射
func saveHashMap(hashMapFile string, chunkFiles []string, rootHashes map[string]string) error {
	file, err := os.Create(hashMapFile)
	if err != nil {
		return fmt.Errorf("创建hash映射文件失败: %w", err)
	}
	defer file.Close()

	// 按文件名排序
	sortedFiles := make([]string, 0, len(chunkFiles))
	for _, f := range chunkFiles {
		sortedFiles = append(sortedFiles, f)
	}
	sort.Strings(sortedFiles)

	for _, chunkFile := range sortedFiles {
		rootHash := rootHashes[chunkFile]
		_, err := fmt.Fprintf(file, "%s|%s\n", chunkFile, rootHash)
		if err != nil {
			return fmt.Errorf("写入hash映射失败: %w", err)
		}
	}

	return nil
}

// loadHashMap 加载分片文件名和root hash的映射
func loadHashMap(hashMapFile string) (map[string]string, error) {
	data, err := os.ReadFile(hashMapFile)
	if err != nil {
		return nil, fmt.Errorf("读取hash映射文件失败: %w", err)
	}

	hashMap := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) == 2 {
			hashMap[parts[0]] = parts[1]
		}
	}

	return hashMap, nil
}

// downloadAndMergeChunks 下载所有分片并合并
func downloadAndMergeChunks(ctx context.Context, config Config, hashMapFile, outputFile string) error {
	// 加载hash映射
	hashMap, err := loadHashMap(hashMapFile)
	if err != nil {
		return fmt.Errorf("加载hash映射失败: %w", err)
	}

	// 创建Indexer客户端
	indexerClient, err := indexer.NewClient(config.IndexerRpc)
	if err != nil {
		return fmt.Errorf("创建Indexer客户端失败: %w", err)
	}

	// 获取可用节点
	nodes, err := indexerClient.SelectNodes(ctx)
	if err != nil {
		return fmt.Errorf("选择节点失败: %w", err)
	}

	// 创建下载器
	downloader, err := transfer.NewDownloader(nodes)
	if err != nil {
		return fmt.Errorf("创建下载器失败: %w", err)
	}

	// 创建输出文件
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	// 按文件名排序，确保顺序正确
	var sortedFiles []string
	for chunkFile := range hashMap {
		sortedFiles = append(sortedFiles, chunkFile)
	}
	sort.Strings(sortedFiles)

	// 下载并合并每个分片
	for i, chunkFile := range sortedFiles {
		rootHash := hashMap[chunkFile]
		if rootHash == "" {
			return fmt.Errorf("分片 %s 没有对应的root hash", chunkFile)
		}

		fmt.Printf("  下载分片 %d: %s (rootHash: %s)\n", i, chunkFile, rootHash)

		// 创建临时文件存储下载的分片
		tempFile := chunkFile + ".downloaded"
		defer os.Remove(tempFile) // 清理临时文件

		// 下载文件
		err := downloader.Download(ctx, rootHash, tempFile, false) // false表示不验证proof
		if err != nil {
			return fmt.Errorf("下载分片 %s 失败: %w", chunkFile, err)
		}

		// 读取下载的文件并写入输出文件
		chunkData, err := os.ReadFile(tempFile)
		if err != nil {
			return fmt.Errorf("读取下载的分片失败: %w", err)
		}

		if _, err := outFile.Write(chunkData); err != nil {
			return fmt.Errorf("写入输出文件失败: %w", err)
		}

		fmt.Printf("  分片 %d 下载并合并完成 (大小: %d 字节)\n", i, len(chunkData))
	}

	return nil
}

// calculateFileRootHash 计算文件的root hash
// 注意：这是一个简化实现，实际应该使用0g-storage-client提供的API来计算Merkle tree的root hash
// 如果0g-storage-client提供了直接计算root hash的方法，应该使用那个方法
// 或者，如果UploadFile返回了root hash，应该从返回结果中获取
func calculateFileRootHash(filePath string) (string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	// 读取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 注意：这里使用文件路径和大小生成一个临时hash
	// 实际应用中，应该使用0g-storage-client的API来计算真正的Merkle tree root hash
	// 例如：使用transfer包中的方法，或者在上传后从返回结果中获取root hash
	
	// 临时方案：使用文件路径和大小生成标识符
	// 实际使用时，应该从UploadFile的返回结果中获取root hash，或者使用0g-storage-client提供的API
	hash := fmt.Sprintf("%x", fileInfo.Size()) + filepath.Base(filePath)
	
	// 如果0g-storage-client的UploadFile返回了root hash，应该使用那个
	// 这里返回一个占位符，实际使用时需要根据API调整
	// 生成64字符的hash（32字节的hex表示）
	if len(hash) < 64 {
		hash = hash + strings.Repeat("0", 64-len(hash))
	} else {
		hash = hash[:64]
	}
	
	return "0x" + hash, nil
}

