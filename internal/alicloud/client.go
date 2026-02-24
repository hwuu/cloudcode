// Package alicloud 封装阿里云 SDK 调用，提供 ECS/VPC/STS/EIP 等资源的创建、查询和删除操作。
// 所有函数通过接口（ECSAPI/VPCAPI/STSAPI）接收 SDK 客户端，支持 mock 测试。
package alicloud

import (
	"os"

	"github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
	"github.com/hwuu/cloudcode/internal/config"
)

const (
	DefaultRegion   = "ap-southeast-1" // 默认区域：新加坡
	EnvAccessKeyID  = "ALICLOUD_ACCESS_KEY_ID"
	EnvAccessSecret = "ALICLOUD_ACCESS_KEY_SECRET"
)

// Config 阿里云 SDK 认证配置
type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	RegionID        string
}

// LoadConfig 加载阿里云配置。
// 优先级：环境变量 → ~/.cloudcode/credentials → 报错提示 cloudcode init。
func LoadConfig() (*Config, error) {
	// 优先从环境变量加载
	accessKeyID := os.Getenv(EnvAccessKeyID)
	accessKeySecret := os.Getenv(EnvAccessSecret)

	if accessKeyID != "" && accessKeySecret != "" {
		regionID := os.Getenv("ALICLOUD_REGION")
		if regionID == "" {
			regionID = DefaultRegion
		}
		return &Config{
			AccessKeyID:     accessKeyID,
			AccessKeySecret: accessKeySecret,
			RegionID:        regionID,
		}, nil
	}

	// 从 credentials 文件加载
	cred, err := config.LoadCredentials()
	if err != nil {
		// 环境变量部分设置但不完整时，给出具体提示
		if accessKeyID != "" || accessKeySecret != "" {
			if accessKeyID == "" {
				return nil, ErrMissingAccessKeyID
			}
			return nil, ErrMissingAccessKeySecret
		}
		return nil, ErrMissingConfig
	}

	regionID := cred.Region
	if regionID == "" {
		regionID = DefaultRegion
	}

	return &Config{
		AccessKeyID:     cred.AccessKeyID,
		AccessKeySecret: cred.AccessKeySecret,
		RegionID:        regionID,
	}, nil
}

// LoadConfigFromEnv 从环境变量加载阿里云配置（向后兼容）。
func LoadConfigFromEnv() (*Config, error) {
	accessKeyID := os.Getenv(EnvAccessKeyID)
	accessKeySecret := os.Getenv(EnvAccessSecret)

	if accessKeyID == "" {
		return nil, ErrMissingAccessKeyID
	}
	if accessKeySecret == "" {
		return nil, ErrMissingAccessKeySecret
	}

	regionID := os.Getenv("ALICLOUD_REGION")
	if regionID == "" {
		regionID = DefaultRegion
	}

	return &Config{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		RegionID:        regionID,
	}, nil
}

// Clients 持有所有阿里云 SDK 客户端实例
type Clients struct {
	ECS *ecsclient.Client
	VPC *vpcclient.Client
	STS *stsclient.Client
}

// NewClients 使用统一配置初始化 ECS/VPC/STS 三个 SDK 客户端
func NewClients(cfg *Config) (*Clients, error) {
	openAPIConfig := &client.Config{
		AccessKeyId:     &cfg.AccessKeyID,
		AccessKeySecret: &cfg.AccessKeySecret,
		RegionId:        &cfg.RegionID,
	}

	ecsCli, err := ecsclient.NewClient(openAPIConfig)
	if err != nil {
		return nil, err
	}

	vpcCli, err := vpcclient.NewClient(openAPIConfig)
	if err != nil {
		return nil, err
	}

	stsCli, err := stsclient.NewClient(openAPIConfig)
	if err != nil {
		return nil, err
	}

	return &Clients{
		ECS: ecsCli,
		VPC: vpcCli,
		STS: stsCli,
	}, nil
}

// ClientInterface 统一的客户端访问接口，用于依赖注入
type ClientInterface interface {
	STSClient() STSAPI
	ECSClient() ECSAPI
	VPCClient() VPCAPI
}

type clientWrapper struct {
	clients *Clients
}

// NewClientWrapper 将 Clients 包装为 ClientInterface
func NewClientWrapper(clients *Clients) ClientInterface {
	return &clientWrapper{clients: clients}
}

func (w *clientWrapper) STSClient() STSAPI {
	return w.clients.STS
}

func (w *clientWrapper) ECSClient() ECSAPI {
	return w.clients.ECS
}

func (w *clientWrapper) VPCClient() VPCAPI {
	return w.clients.VPC
}
