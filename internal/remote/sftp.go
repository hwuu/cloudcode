package remote

import (
	"fmt"
)

// SFTPClient 抽象 SFTP 文件上传，支持 mock 测试
type SFTPClient interface {
	UploadFile(localContent []byte, remotePath string) error
	Close() error
}

// UploadFiles 批量上传文件，任一失败立即返回错误
func UploadFiles(client SFTPClient, files map[string][]byte) error {
	for remotePath, content := range files {
		if err := client.UploadFile(content, remotePath); err != nil {
			return fmt.Errorf("failed to upload %s: %w", remotePath, err)
		}
	}
	return nil
}
