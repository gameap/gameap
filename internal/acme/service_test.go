package acme_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/gameap/gameap/internal/acme"
	"github.com/gameap/gameap/internal/acme/locker"
	"github.com/gameap/gameap/internal/acme/storage"
	"github.com/gameap/gameap/internal/files"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/registration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClient struct {
	registerCalls atomic.Int32
	obtainCalls   atomic.Int32
	renewCalls    atomic.Int32
	obtainFn      func() (*certificate.Resource, error)
	renewFn       func(certificate.Resource) (*certificate.Resource, error)
}

func (c *fakeClient) Register(_ context.Context, _ bool) (*registration.Resource, error) {
	c.registerCalls.Add(1)

	return &registration.Resource{URI: "https://test/reg/1"}, nil
}

func (c *fakeClient) ObtainCertificate(_ context.Context, _ certificate.ObtainRequest) (*certificate.Resource, error) {
	c.obtainCalls.Add(1)

	return c.obtainFn()
}

func (c *fakeClient) RenewCertificate(_ context.Context, res certificate.Resource) (*certificate.Resource, error) {
	c.renewCalls.Add(1)

	return c.renewFn(res)
}

type fakeRegistry struct{}

func (fakeRegistry) Resolve(_ context.Context, _ string) (challenge.Provider, error) {
	return fakeProvider{}, nil
}

type fakeProvider struct{}

func (fakeProvider) Present(_, _, _ string) error { return nil }
func (fakeProvider) CleanUp(_, _, _ string) error { return nil }

func TestService_Status_DefaultsToPending(t *testing.T) {
	t.Parallel()

	svc := newServiceForTest(t, nil)

	st := svc.Status()
	assert.True(t, st.Enabled)
	assert.Equal(t, acme.StatePending, st.State)
	assert.Equal(t, []string{"example.com"}, st.Domains)
}

func TestService_HTTP01Handler_NilForDNS01(t *testing.T) {
	t.Parallel()

	cfg := serviceConfigFor("example.com")
	cfg.ChallengeType = acme.ChallengeDNS01

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	svc := acme.NewService(cfg, st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)

	assert.Nil(t, svc.HTTP01Handler())
}

func TestService_HTTP01Handler_PresentForHTTP01(t *testing.T) {
	t.Parallel()

	cfg := serviceConfigFor("example.com")
	cfg.ChallengeType = acme.ChallengeHTTP01

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	svc := acme.NewService(cfg, st, locker.NewInMemoryLocker(), nil, nil)

	assert.NotNil(t, svc.HTTP01Handler())
}

func TestService_GetCertificate_ReturnsErrorBeforeStart(t *testing.T) {
	t.Parallel()

	svc := newServiceForTest(t, nil)

	cert, err := svc.GetCertificate(nil)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "no certificate available")
}

func TestService_Start_ObtainsAndCachesCertificate(t *testing.T) {
	t.Parallel()

	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) {
			return resource, nil
		},
	}

	svc := newServiceForTest(t, client)

	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	assert.Equal(t, int32(1), client.registerCalls.Load())
	assert.Equal(t, int32(1), client.obtainCalls.Load())

	cert, err := svc.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotNil(t, cert.Leaf)
	assert.Equal(t, "example.com", cert.Leaf.Subject.CommonName)

	st := svc.Status()
	assert.Equal(t, acme.StateActive, st.State)
	assert.WithinDuration(t, time.Now(), st.LastRenewalAt, 5*time.Second)
}

func TestService_Start_FailsWhenObtainFails(t *testing.T) {
	t.Parallel()

	leUnreachableErr := errors.New("LE unreachable")

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) {
			return nil, leUnreachableErr
		},
	}

	svc := newServiceForTest(t, client)

	err := svc.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LE unreachable")

	st := svc.Status()
	assert.Equal(t, acme.StateFailed, st.State)
	assert.Contains(t, st.LastError, "LE unreachable")
}

func TestService_ForceRenew_ReplacesCertificate(t *testing.T) {
	t.Parallel()

	original := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))
	renewed := generateTestResource(t, "example.com", time.Now().Add(90*24*time.Hour))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return original, nil },
		renewFn:  func(_ certificate.Resource) (*certificate.Resource, error) { return renewed, nil },
	}

	svc := newServiceForTest(t, client)

	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	originalCert, err := svc.GetCertificate(nil)
	require.NoError(t, err)
	originalNotAfter := originalCert.Leaf.NotAfter

	require.NoError(t, svc.ForceRenew(context.Background()))
	assert.Equal(t, int32(1), client.renewCalls.Load())

	renewedCert, err := svc.GetCertificate(nil)
	require.NoError(t, err)
	assert.True(t, renewedCert.Leaf.NotAfter.After(originalNotAfter),
		"expected renewed cert to expire later than original")
}

func TestService_Start_LoadsCachedCertificateWhenFresh(t *testing.T) {
	t.Parallel()

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	require.NoError(t, st.SaveResource(context.Background(), "example.com", resource))
	require.NoError(t, st.SaveAccount(context.Background(), &acme.Account{
		Email:        "ops@example.com",
		PrivateKey:   testAccountKeyPEM(t),
		Registration: []byte(`{"uri":"https://test/reg"}`),
	}))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) {
			t.Fatal("Obtain should not be called when storage has fresh certificate")

			return nil, nil
		},
	}

	svc := acme.NewService(serviceConfigFor("example.com"), st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	assert.Equal(t, int32(0), client.obtainCalls.Load())
}

func newServiceForTest(t *testing.T, client *fakeClient) *acme.Service {
	t.Helper()

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	svc := acme.NewService(serviceConfigFor("example.com"), st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)

	if client != nil {
		svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
			return client, nil
		})
	}

	return svc
}

func serviceConfigFor(domain string) acme.ServiceConfig { //nolint:unparam
	return acme.ServiceConfig{
		ChallengeType:        acme.ChallengeDNS01,
		Email:                "ops@example.com",
		Domains:              []string{domain},
		DirectoryURL:         "https://test.acme/dir",
		DNSProvider:          "fake",
		RenewalThreshold:     30 * 24 * time.Hour,
		RenewalCheckInterval: 1 * time.Hour,
		PropagationTimeout:   2 * time.Minute,
		LockTTL:              30 * time.Second,
	}
}

func generateTestResource(t *testing.T, cn string, notAfter time.Time) *certificate.Resource { //nolint:unparam
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{cn},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return &certificate.Resource{
		Domain:            cn,
		CertURL:           "https://test/cert",
		PrivateKey:        keyPEM,
		Certificate:       certPEM,
		IssuerCertificate: certPEM,
	}
}

func testAccountKeyPEM(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// fakeRegisterClient lets a test override Register/Obtain behaviour
// independently. obtainFn defaults to a small valid resource if not set.
type fakeRegisterClient struct {
	registerCalls atomic.Int32
	obtainCalls   atomic.Int32
	registerFn    func() (*registration.Resource, error)
	obtainFn      func() (*certificate.Resource, error)
	renewFn       func(certificate.Resource) (*certificate.Resource, error)
}

func (c *fakeRegisterClient) Register(_ context.Context, _ bool) (*registration.Resource, error) {
	c.registerCalls.Add(1)
	if c.registerFn != nil {
		return c.registerFn()
	}

	return &registration.Resource{URI: "https://test/reg/1"}, nil
}

func (c *fakeRegisterClient) ObtainCertificate(
	_ context.Context, _ certificate.ObtainRequest,
) (*certificate.Resource, error) {
	c.obtainCalls.Add(1)

	return c.obtainFn()
}

func (c *fakeRegisterClient) RenewCertificate(
	_ context.Context, res certificate.Resource,
) (*certificate.Resource, error) {
	if c.renewFn == nil {
		return nil, errors.New("renewFn not set")
	}

	return c.renewFn(res)
}

// =============================================================================
// Renewal loop tests (Tier 1B)
// =============================================================================

func TestService_RenewalLoop_TriggersRenewalWhenThresholdCrossed(t *testing.T) {
	t.Parallel()

	// ARRANGE: cert expires in 1 hour, threshold is 2 hours → loop must renew.
	expiring := generateTestResource(t, "example.com", time.Now().Add(1*time.Hour))
	renewed := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return expiring, nil },
		renewFn:  func(_ certificate.Resource) (*certificate.Resource, error) { return renewed, nil },
	}

	cfg := serviceConfigFor("example.com")
	cfg.RenewalThreshold = 2 * time.Hour
	cfg.RenewalCheckInterval = 30 * time.Millisecond

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	svc := acme.NewService(cfg, st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT
	assert.Eventually(t, func() bool {
		return client.renewCalls.Load() >= 1
	}, 2*time.Second, 20*time.Millisecond,
		"renewal loop must invoke RenewCertificate when cert is past threshold")

	// The renewed certificate is installed after RenewCertificate returns
	// (save → parse → apply), so the swap lags the call counter.
	assert.Eventually(t, func() bool {
		cert, err := svc.GetCertificate(nil)

		return err == nil && cert.Leaf != nil &&
			cert.Leaf.NotAfter.After(time.Now().Add(2*time.Hour))
	}, 2*time.Second, 20*time.Millisecond,
		"served cert NotAfter must be far in the future after renewal")
}

func TestService_RenewalLoop_SkipsRenewalWhenCertFresh(t *testing.T) {
	t.Parallel()

	// ARRANGE: cert expires in 60d, threshold is 30d → loop must NOT renew.
	fresh := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return fresh, nil },
		renewFn: func(_ certificate.Resource) (*certificate.Resource, error) {
			t.Fatal("RenewCertificate must not be called when cert is fresh")

			return nil, nil //nolint:nilnil
		},
	}

	cfg := serviceConfigFor("example.com")
	cfg.RenewalThreshold = 30 * 24 * time.Hour
	cfg.RenewalCheckInterval = 30 * time.Millisecond

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	svc := acme.NewService(cfg, st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	time.Sleep(150 * time.Millisecond)

	// ASSERT
	assert.Equal(t, int32(0), client.renewCalls.Load(),
		"renew must not be called when cert is fresher than threshold")
	assert.Equal(t, int32(1), client.obtainCalls.Load(),
		"only the initial Obtain must run")
}

func TestService_Stop_CancelsRunningLoop(t *testing.T) {
	t.Parallel()

	// ARRANGE
	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return resource, nil },
	}

	cfg := serviceConfigFor("example.com")
	cfg.RenewalCheckInterval = 20 * time.Millisecond

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")
	svc := acme.NewService(cfg, st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	require.NoError(t, svc.Start(context.Background()))

	// ACT: wait at least one tick, then stop and ensure no further obtain calls.
	time.Sleep(60 * time.Millisecond)
	require.NoError(t, svc.Stop(context.Background()))

	postStop := client.obtainCalls.Load()
	time.Sleep(80 * time.Millisecond)

	// ASSERT
	assert.Equal(t, postStop, client.obtainCalls.Load(),
		"no obtain calls must happen after Stop returns")
}

func TestService_LoadCached_WithInvalidPEMTriggersReissue(t *testing.T) {
	t.Parallel()

	// ARRANGE: storage holds a corrupted resource; service must fall back to Obtain.
	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")

	corrupt := &certificate.Resource{
		Domain:      "example.com",
		Certificate: []byte("NOT A PEM CERT"),
		PrivateKey:  []byte("NOT A PEM KEY"),
	}
	require.NoError(t, st.SaveResource(context.Background(), "example.com", corrupt))
	require.NoError(t, st.SaveAccount(context.Background(), &acme.Account{
		Email:        "ops@example.com",
		PrivateKey:   testAccountKeyPEM(t),
		Registration: []byte(`{"uri":"https://test/reg"}`),
	}))

	freshRes := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return freshRes, nil },
	}

	svc := acme.NewService(serviceConfigFor("example.com"), st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	err := svc.Start(context.Background())
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT: corrupt cached cert must trigger Obtain rather than fail Start.
	require.NoError(t, err)
	assert.Equal(t, int32(1), client.obtainCalls.Load(),
		"Obtain must run when cached resource is invalid PEM")
}

func TestService_LoadCached_WithExpiredCertTriggersImmediateRenewal(t *testing.T) {
	t.Parallel()

	// ARRANGE: stored cert is already expired; service must reissue immediately.
	expired := generateTestResource(t, "example.com", time.Now().Add(-1*time.Hour))

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")

	require.NoError(t, st.SaveResource(context.Background(), "example.com", expired))
	require.NoError(t, st.SaveAccount(context.Background(), &acme.Account{
		Email:        "ops@example.com",
		PrivateKey:   testAccountKeyPEM(t),
		Registration: []byte(`{"uri":"https://test/reg"}`),
	}))

	fresh := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))
	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return fresh, nil },
	}

	svc := acme.NewService(serviceConfigFor("example.com"), st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT
	assert.Equal(t, int32(1), client.obtainCalls.Load(),
		"Obtain must run when cached cert is past threshold")

	cert, err := svc.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert.Leaf)
	assert.True(t, cert.Leaf.NotAfter.After(time.Now().Add(30*24*time.Hour)),
		"served cert must be the freshly obtained one, not the expired stored one")
}

func TestService_Status_ReportsNotBeforeNotAfterAfterObtain(t *testing.T) {
	t.Parallel()

	// ARRANGE: cert with explicit validity window we can match against Status.
	notAfter := time.Now().Add(60 * 24 * time.Hour).Truncate(time.Second)
	resource := generateTestResource(t, "example.com", notAfter)

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return resource, nil },
	}

	svc := newServiceForTest(t, client)

	// ACT
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT
	st := svc.Status()
	assert.WithinDuration(t, notAfter, st.NotAfter, time.Second,
		"Status.NotAfter must reflect the certificate leaf's NotAfter")
	assert.False(t, st.NotBefore.IsZero(), "Status.NotBefore must be set after Obtain")
	assert.True(t, st.NotBefore.Before(st.NotAfter),
		"NotBefore must precede NotAfter")
}

// =============================================================================
// ensureAccount tests (Tier 2 D)
// =============================================================================

func TestService_EnsureAccount_CreatesNewAccountWhenStorageEmpty(t *testing.T) {
	t.Parallel()

	// ARRANGE
	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")

	client := &fakeRegisterClient{
		obtainFn: func() (*certificate.Resource, error) { return resource, nil },
	}

	svc := acme.NewService(serviceConfigFor("example.com"), st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT
	assert.Equal(t, int32(1), client.registerCalls.Load(),
		"Register must run when storage has no account")

	has, err := st.HasAccount(context.Background(), "ops@example.com")
	require.NoError(t, err)
	assert.True(t, has, "account must be persisted to storage after Register")
}

func TestService_EnsureAccount_LoadsExistingAccountWithoutRegistering(t *testing.T) {
	t.Parallel()

	// ARRANGE: prepopulate storage, ensure factory returns a client that does NOT register.
	fm := files.NewInMemoryFileManager()
	st := storage.NewFileStorage(fm, "acme")

	require.NoError(t, st.SaveAccount(context.Background(), &acme.Account{
		Email:        "ops@example.com",
		PrivateKey:   testAccountKeyPEM(t),
		Registration: []byte(`{"uri":"https://test/reg/preexisting"}`),
	}))

	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))
	client := &fakeRegisterClient{
		obtainFn: func() (*certificate.Resource, error) { return resource, nil },
	}

	svc := acme.NewService(serviceConfigFor("example.com"), st, locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT
	assert.Equal(t, int32(0), client.registerCalls.Load(),
		"Register must NOT be called when account already exists in storage")
}

func TestService_EnsureAccount_PropagatesRegistrationError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	regErr := errors.New("registration rejected by CA")
	client := &fakeRegisterClient{
		registerFn: func() (*registration.Resource, error) { return nil, regErr },
	}

	svc := acme.NewService(serviceConfigFor("example.com"), storage.NewFileStorage(files.NewInMemoryFileManager(), "acme"), locker.NewInMemoryLocker(), fakeRegistry{}, nil)
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return client, nil
	})

	// ACT
	err := svc.Start(context.Background())

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registration rejected by CA")

	// ensureAccount returns an error before tickRenewal/setFailed is called from
	// ensureCertificate, so status remains StatePending. The contract: Start
	// returns the error wrapped with "failed to ensure ACME account".
	assert.Contains(t, err.Error(), "ACME account")
}

// =============================================================================
// State transitions (Tier 2 D)
// =============================================================================

func TestService_Status_SetsFailedWithLastErrorOnObtainError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	obtainErr := errors.New("CA validation failed")

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return nil, obtainErr },
	}

	svc := newServiceForTest(t, client)

	// ACT
	err := svc.Start(context.Background())

	// ASSERT
	require.Error(t, err)
	st := svc.Status()
	assert.Equal(t, acme.StateFailed, st.State, "state must transition to Failed on obtain error")
	assert.Contains(t, st.LastError, "CA validation failed",
		"LastError must contain the underlying error message")
}

func TestService_Status_ClearsLastErrorOnSuccessfulRenew(t *testing.T) {
	t.Parallel()

	// ARRANGE: first Obtain fails, ForceRenew succeeds.
	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	var calls atomic.Int32
	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) {
			n := calls.Add(1)
			if n == 1 {
				return nil, errors.New("transient obtain failure")
			}

			return resource, nil
		},
	}

	svc := newServiceForTest(t, client)

	startErr := svc.Start(context.Background())
	require.Error(t, startErr)
	require.Equal(t, acme.StateFailed, svc.Status().State)
	require.NotEmpty(t, svc.Status().LastError)

	// ACT: ForceRenew with no current resource falls through to Obtain.
	require.NoError(t, svc.ForceRenew(context.Background()))

	// ASSERT
	st := svc.Status()
	assert.Equal(t, acme.StateActive, st.State)
	assert.Empty(t, st.LastError, "LastError must be cleared once a new cert is in place")
}

// =============================================================================
// Concurrent operations (Tier 2 D)
// =============================================================================

func TestService_ForceRenew_ConcurrentSerializesViaLocker(t *testing.T) {
	t.Parallel()

	// ARRANGE: lots of parallel ForceRenew callers; the locker must serialize
	// them so RenewCertificate is not called concurrently. With the in-memory
	// locker this means losing callers see ErrLocked while one caller proceeds.
	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))

	var inFlight atomic.Int32
	var maxConcurrent atomic.Int32

	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return resource, nil },
		renewFn: func(_ certificate.Resource) (*certificate.Resource, error) {
			n := inFlight.Add(1)
			defer inFlight.Add(-1)

			for {
				cur := maxConcurrent.Load()
				if n <= cur || maxConcurrent.CompareAndSwap(cur, n) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)

			return resource, nil
		},
	}

	svc := newServiceForTest(t, client)

	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ACT
	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for range goroutines {
		wg.Go(func() {
			errs <- svc.ForceRenew(context.Background())
		})
	}

	wg.Wait()
	close(errs)

	// ASSERT: at least one ForceRenew must succeed; the rest may bounce off
	// ErrLocked. The critical invariant is that no two RenewCertificate calls
	// overlap — maxConcurrent must stay at 1.
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}

	assert.GreaterOrEqual(t, successes, 1, "at least one ForceRenew must succeed")
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"locker must keep RenewCertificate strictly serial; observed concurrency was %d",
		maxConcurrent.Load())
}

func TestService_Status_SafeUnderConcurrentAccess(t *testing.T) {
	t.Parallel()

	// ARRANGE
	resource := generateTestResource(t, "example.com", time.Now().Add(60*24*time.Hour))
	client := &fakeClient{
		obtainFn: func() (*certificate.Resource, error) { return resource, nil },
		renewFn: func(_ certificate.Resource) (*certificate.Resource, error) {
			return resource, nil
		},
	}

	svc := newServiceForTest(t, client)
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ACT: parallel readers + writers; under -race this surfaces any data race
	// on the status struct.
	const readers = 20

	var wg sync.WaitGroup
	for range readers {
		wg.Go(func() {
			for range 50 {
				_ = svc.Status()
			}
		})
	}

	wg.Go(func() {
		_ = svc.ForceRenew(context.Background())
	})

	wg.Wait()

	// ASSERT: completion without -race tripping is the contract; sanity check
	// the final state is still readable.
	st := svc.Status()
	assert.NotEmpty(t, st.Domains)
}
