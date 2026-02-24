package unit

import (
	"testing"

	dnsclient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hwuu/cloudcode/internal/alicloud"
)

func TestFindBaseDomain_Simple(t *testing.T) {
	domains := []string{"example.com", "other.org"}
	base, rr, err := alicloud.FindBaseDomain("oc.example.com", domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "example.com" {
		t.Errorf("expected base 'example.com', got '%s'", base)
	}
	if rr != "oc" {
		t.Errorf("expected rr 'oc', got '%s'", rr)
	}
}

func TestFindBaseDomain_MultiLevel(t *testing.T) {
	domains := []string{"example.co.uk"}
	base, rr, err := alicloud.FindBaseDomain("oc.example.co.uk", domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "example.co.uk" {
		t.Errorf("expected base 'example.co.uk', got '%s'", base)
	}
	if rr != "oc" {
		t.Errorf("expected rr 'oc', got '%s'", rr)
	}
}

func TestFindBaseDomain_SubSubDomain(t *testing.T) {
	domains := []string{"example.com"}
	base, rr, err := alicloud.FindBaseDomain("dev.oc.example.com", domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "example.com" {
		t.Errorf("expected base 'example.com', got '%s'", base)
	}
	if rr != "dev.oc" {
		t.Errorf("expected rr 'dev.oc', got '%s'", rr)
	}
}

func TestFindBaseDomain_ExactMatch(t *testing.T) {
	domains := []string{"example.com"}
	base, rr, err := alicloud.FindBaseDomain("example.com", domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "example.com" {
		t.Errorf("expected base 'example.com', got '%s'", base)
	}
	if rr != "@" {
		t.Errorf("expected rr '@', got '%s'", rr)
	}
}

func TestFindBaseDomain_NotFound(t *testing.T) {
	domains := []string{"example.com", "other.org"}
	_, _, err := alicloud.FindBaseDomain("oc.unknown.net", domains)
	if err == nil {
		t.Error("expected error for unmatched domain")
	}
}

func TestFindBaseDomain_TrailingDot(t *testing.T) {
	domains := []string{"example.com"}
	base, rr, err := alicloud.FindBaseDomain("oc.example.com.", domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "example.com" {
		t.Errorf("expected base 'example.com', got '%s'", base)
	}
	if rr != "oc" {
		t.Errorf("expected rr 'oc', got '%s'", rr)
	}
}

func TestFindBaseDomain_EmptyDomains(t *testing.T) {
	_, _, err := alicloud.FindBaseDomain("oc.example.com", nil)
	if err == nil {
		t.Error("expected error for empty domains list")
	}
}

func TestEnsureDNSRecord_CreateNew(t *testing.T) {
	addCalled := false
	mock := &MockDnsAPI{
		DescribeDomainRecordsFunc: func(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error) {
			// 无现有记录
			return &dnsclient.DescribeDomainRecordsResponse{
				Body: &dnsclient.DescribeDomainRecordsResponseBody{
					DomainRecords: &dnsclient.DescribeDomainRecordsResponseBodyDomainRecords{
						Record: []*dnsclient.DescribeDomainRecordsResponseBodyDomainRecordsRecord{},
					},
				},
			}, nil
		},
		AddDomainRecordFunc: func(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error) {
			addCalled = true
			if *req.RR != "oc" {
				t.Errorf("expected RR 'oc', got '%s'", *req.RR)
			}
			if *req.Value != "1.2.3.4" {
				t.Errorf("expected value '1.2.3.4', got '%s'", *req.Value)
			}
			return &dnsclient.AddDomainRecordResponse{}, nil
		},
	}

	err := alicloud.EnsureDNSRecord(mock, "example.com", "oc", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !addCalled {
		t.Error("expected AddDomainRecord to be called")
	}
}

func TestEnsureDNSRecord_UpdateExisting(t *testing.T) {
	updateCalled := false
	recordID := "record-123"
	mock := &MockDnsAPI{
		DescribeDomainRecordsFunc: func(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error) {
			return &dnsclient.DescribeDomainRecordsResponse{
				Body: &dnsclient.DescribeDomainRecordsResponseBody{
					DomainRecords: &dnsclient.DescribeDomainRecordsResponseBodyDomainRecords{
						Record: []*dnsclient.DescribeDomainRecordsResponseBodyDomainRecordsRecord{
							{
								RecordId: &recordID,
								RR:       tea.String("oc"),
								Type:     tea.String("A"),
								Value:    tea.String("5.6.7.8"), // 旧 IP
							},
						},
					},
				},
			}, nil
		},
		UpdateDomainRecordFunc: func(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error) {
			updateCalled = true
			if *req.RecordId != recordID {
				t.Errorf("expected record ID '%s', got '%s'", recordID, *req.RecordId)
			}
			if *req.Value != "1.2.3.4" {
				t.Errorf("expected value '1.2.3.4', got '%s'", *req.Value)
			}
			return &dnsclient.UpdateDomainRecordResponse{}, nil
		},
	}

	err := alicloud.EnsureDNSRecord(mock, "example.com", "oc", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("expected UpdateDomainRecord to be called")
	}
}

func TestEnsureDNSRecord_SameIP_NoOp(t *testing.T) {
	mock := &MockDnsAPI{
		DescribeDomainRecordsFunc: func(req *dnsclient.DescribeDomainRecordsRequest) (*dnsclient.DescribeDomainRecordsResponse, error) {
			return &dnsclient.DescribeDomainRecordsResponse{
				Body: &dnsclient.DescribeDomainRecordsResponseBody{
					DomainRecords: &dnsclient.DescribeDomainRecordsResponseBodyDomainRecords{
						Record: []*dnsclient.DescribeDomainRecordsResponseBodyDomainRecordsRecord{
							{
								RecordId: tea.String("record-123"),
								RR:       tea.String("oc"),
								Type:     tea.String("A"),
								Value:    tea.String("1.2.3.4"), // 相同 IP
							},
						},
					},
				},
			}, nil
		},
		UpdateDomainRecordFunc: func(req *dnsclient.UpdateDomainRecordRequest) (*dnsclient.UpdateDomainRecordResponse, error) {
			t.Error("UpdateDomainRecord should not be called when IP is the same")
			return nil, nil
		},
		AddDomainRecordFunc: func(req *dnsclient.AddDomainRecordRequest) (*dnsclient.AddDomainRecordResponse, error) {
			t.Error("AddDomainRecord should not be called when record exists")
			return nil, nil
		},
	}

	err := alicloud.EnsureDNSRecord(mock, "example.com", "oc", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
