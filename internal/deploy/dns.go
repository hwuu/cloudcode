package deploy

import (
	"fmt"
	"net"
	"time"
)

// waitForDNS 轮询 DNS 解析，直到域名解析到指定 IP 或超时。
// 用于非阿里云域名场景，用户手动配置 DNS 后等待生效。
func waitForDNS(domain, expectedIP string, timeout time.Duration, printf func(string, ...interface{})) error {
	deadline := time.Now().Add(timeout)
	interval := 5 * time.Second

	for time.Now().Before(deadline) {
		addrs, err := net.LookupHost(domain)
		if err == nil {
			for _, addr := range addrs {
				if addr == expectedIP {
					return nil
				}
			}
		}
		printf("  等待 DNS 生效... (%s → 期望 %s)\n", domain, expectedIP)
		time.Sleep(interval)
	}

	return fmt.Errorf("DNS 解析超时（%v），%s 未解析到 %s", timeout, domain, expectedIP)
}
