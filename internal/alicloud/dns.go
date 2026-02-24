package alicloud

import (
	"fmt"
	"strings"

	dnsclient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	"github.com/alibabacloud-go/tea/tea"
)

// FindBaseDomain 从用户域名列表中匹配 baseDomain 和主机记录。
// 通过后缀匹配避免手动解析多级后缀（如 .co.uk）。
//
// Input:  fullDomain="oc.example.com", domains=["example.com", "other.org"]
// Output: baseDomain="example.com", rr="oc"
func FindBaseDomain(fullDomain string, domains []string) (baseDomain, rr string, err error) {
	fullDomain = strings.TrimSuffix(fullDomain, ".")
	for _, d := range domains {
		suffix := "." + d
		if strings.HasSuffix(fullDomain, suffix) {
			rr = strings.TrimSuffix(fullDomain, suffix)
			if rr == "" {
				continue
			}
			return d, rr, nil
		}
		if fullDomain == d {
			return d, "@", nil
		}
	}
	return "", "", fmt.Errorf("域名 %s 未在阿里云 DNS 中找到匹配的主域名", fullDomain)
}

// ListDomains 获取用户在阿里云 DNS 中的所有域名
func ListDomains(cli DnsAPI) ([]string, error) {
	var domains []string
	pageNumber := int64(1)
	pageSize := int64(100)

	for {
		req := &dnsclient.DescribeDomainsRequest{
			PageNumber: &pageNumber,
			PageSize:   &pageSize,
		}
		resp, err := cli.DescribeDomains(req)
		if err != nil {
			return nil, fmt.Errorf("查询域名列表失败: %w", err)
		}
		if resp == nil || resp.Body == nil || resp.Body.Domains == nil {
			break
		}
		for _, d := range resp.Body.Domains.Domain {
			if d.DomainName != nil {
				domains = append(domains, *d.DomainName)
			}
		}
		total := int64(0)
		if resp.Body.TotalCount != nil {
			total = *resp.Body.TotalCount
		}
		if pageNumber*pageSize >= total {
			break
		}
		pageNumber++
	}

	return domains, nil
}

// EnsureDNSRecord 创建或更新一条 A 记录。
// 如果记录已存在且 IP 不同则更新，不存在则创建。
func EnsureDNSRecord(cli DnsAPI, baseDomain, rr, ip string) error {
	// 查询现有记录
	req := &dnsclient.DescribeDomainRecordsRequest{
		DomainName: &baseDomain,
		RRKeyWord:  &rr,
		Type:       tea.String("A"),
	}
	resp, err := cli.DescribeDomainRecords(req)
	if err != nil {
		return fmt.Errorf("查询 DNS 记录失败: %w", err)
	}

	// 查找精确匹配的记录
	if resp.Body != nil && resp.Body.DomainRecords != nil {
		for _, record := range resp.Body.DomainRecords.Record {
			if record.RR != nil && *record.RR == rr && record.Type != nil && *record.Type == "A" {
				// 记录已存在
				if record.Value != nil && *record.Value == ip {
					return nil // IP 相同，无需更新
				}
				// IP 不同，更新记录
				updateReq := &dnsclient.UpdateDomainRecordRequest{
					RecordId: record.RecordId,
					RR:       &rr,
					Type:     tea.String("A"),
					Value:    &ip,
				}
				if _, err := cli.UpdateDomainRecord(updateReq); err != nil {
					return fmt.Errorf("更新 DNS 记录失败: %w", err)
				}
				return nil
			}
		}
	}

	// 记录不存在，创建新记录
	addReq := &dnsclient.AddDomainRecordRequest{
		DomainName: &baseDomain,
		RR:         &rr,
		Type:       tea.String("A"),
		Value:      &ip,
	}
	if _, err := cli.AddDomainRecord(addReq); err != nil {
		return fmt.Errorf("创建 DNS 记录失败: %w", err)
	}
	return nil
}
