package alicloud

import (
	"os"

	"github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecsclient "github.com/alibabacloud-go/ecs-20140526/v4/client"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	vpcclient "github.com/alibabacloud-go/vpc-20160428/v6/client"
)

const (
	DefaultRegion   = "ap-southeast-1"
	EnvAccessKeyID  = "ALICLOUD_ACCESS_KEY_ID"
	EnvAccessSecret = "ALICLOUD_ACCESS_KEY_SECRET"
)

type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	RegionID        string
}

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

type Clients struct {
	ECS *ecsclient.Client
	VPC *vpcclient.Client
	STS *stsclient.Client
}

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

type ClientInterface interface {
	STSClient() STSAPI
	ECSClient() ECSAPI
	VPCClient() VPCAPI
}

type clientWrapper struct {
	clients *Clients
}

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
