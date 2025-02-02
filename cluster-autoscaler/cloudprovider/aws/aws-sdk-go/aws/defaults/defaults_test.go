package defaults

import (
	"fmt"
	"os"
	"testing"

	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/aws"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/aws/awserr"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/aws/credentials/endpointcreds"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/aws/request"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/internal/sdktesting"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/aws-sdk-go/internal/shareddefaults"
)

func TestHTTPCredProvider(t *testing.T) {
	origFn := lookupHostFn
	defer func() { lookupHostFn = origFn }()

	lookupHostFn = func(host string) ([]string, error) {
		m := map[string]struct {
			Addrs []string
			Err   error
		}{
			"localhost":         {Addrs: []string{"::1", "127.0.0.1"}},
			"actuallylocal":     {Addrs: []string{"127.0.0.2"}},
			"notlocal":          {Addrs: []string{"::1", "127.0.0.1", "192.168.1.10"}},
			"www.example.com":   {Addrs: []string{"10.10.10.10"}},
			"www.eks.legit.com": {Addrs: []string{"fd00:ec2::23"}},
			"www.eks.scary.com": {Addrs: []string{"fd00:ec3::23"}},
		}

		h, ok := m[host]
		if !ok {
			t.Fatalf("unknown host in test, %v", host)
			return nil, fmt.Errorf("unknown host")
		}

		return h.Addrs, h.Err
	}

	cases := []struct {
		Host      string
		AuthToken string
		Fail      bool
	}{
		{Host: "localhost", Fail: false},
		{Host: "actuallylocal", Fail: false},
		{Host: "127.0.0.1", Fail: false},
		{Host: "127.1.1.1", Fail: false},
		{Host: "[::1]", Fail: false},
		{Host: "www.example.com", Fail: true},
		{Host: "169.254.170.2", Fail: false},
		{Host: "169.254.170.23", Fail: false},
		{Host: "[fd00:ec2::23]", Fail: false},
		{Host: "[fd00:ec2:0::23]", Fail: false},
		{Host: "[fd00:ec2:0:1::23]", Fail: true},
		{Host: "www.eks.legit.com", Fail: false},
		{Host: "www.eks.scary.com", Fail: true},
		{Host: "localhost", Fail: false, AuthToken: "Basic abc123"},
	}

	restoreEnvFn := sdktesting.StashEnv()
	defer restoreEnvFn()

	for i, c := range cases {
		u := fmt.Sprintf("http://%s/abc/123", c.Host)
		os.Setenv(httpProviderEnvVar, u)
		os.Setenv(httpProviderAuthorizationEnvVar, c.AuthToken)

		provider := RemoteCredProvider(aws.Config{}, request.Handlers{})
		if provider == nil {
			t.Fatalf("%d, expect provider not to be nil, but was", i)
		}

		if c.Fail {
			creds, err := provider.Retrieve()
			if err == nil {
				t.Fatalf("%d, expect error but got none", i)
			} else {
				aerr := err.(awserr.Error)
				if e, a := "CredentialsEndpointError", aerr.Code(); e != a {
					t.Errorf("%d, expect %s error code, got %s", i, e, a)
				}
			}
			if e, a := endpointcreds.ProviderName, creds.ProviderName; e != a {
				t.Errorf("%d, expect %s provider name got %s", i, e, a)
			}
		} else {
			httpProvider := provider.(*endpointcreds.Provider)
			if e, a := u, httpProvider.Client.Endpoint; e != a {
				t.Errorf("%d, expect %q endpoint, got %q", i, e, a)
			}
			if e, a := c.AuthToken, httpProvider.AuthorizationToken; e != a {
				t.Errorf("%d, expect %q auth token, got %q", i, e, a)
			}
		}
	}
}

func TestHTTPAuthTokenFile(t *testing.T) {
	restoreEnvFn := sdktesting.StashEnv()
	defer restoreEnvFn()
	os.Setenv(httpProviderAuthFileEnvVar, "path/to/file")
	os.Setenv(httpProviderEnvVar, "http://169.254.170.23/abc/123")

	provider := RemoteCredProvider(aws.Config{}, request.Handlers{})
	if provider == nil {
		t.Fatalf("expect provider not to be nil, but was")
	}

	httpProvider := provider.(*endpointcreds.Provider)
	if httpProvider == nil {
		t.Fatalf("expect provider not to be nil, but was")
	}

	if httpProvider.AuthorizationTokenProvider == nil {
		t.Fatalf("expect auth token provider no to be nil, but was")
	}
}

func TestECSCredProvider(t *testing.T) {
	restoreEnvFn := sdktesting.StashEnv()
	defer restoreEnvFn()
	os.Setenv(shareddefaults.ECSCredsProviderEnvVar, "/abc/123")

	provider := RemoteCredProvider(aws.Config{}, request.Handlers{})
	if provider == nil {
		t.Fatalf("expect provider not to be nil, but was")
	}

	httpProvider := provider.(*endpointcreds.Provider)
	if httpProvider == nil {
		t.Fatalf("expect provider not to be nil, but was")
	}
	if e, a := "http://169.254.170.2/abc/123", httpProvider.Client.Endpoint; e != a {
		t.Errorf("expect %q endpoint, got %q", e, a)
	}
}

func TestDefaultEC2RoleProvider(t *testing.T) {
	provider := RemoteCredProvider(aws.Config{}, request.Handlers{})
	if provider == nil {
		t.Fatalf("expect provider not to be nil, but was")
	}

	ec2Provider := provider.(*ec2rolecreds.EC2RoleProvider)
	if ec2Provider == nil {
		t.Fatalf("expect provider not to be nil, but was")
	}
	if e, a := "http://169.254.169.254", ec2Provider.Client.Endpoint; e != a {
		t.Errorf("expect %q endpoint, got %q", e, a)
	}
}
