package ponrunner

import (
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/ponrove/configura"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestSetOpenFeatureProviderTestCase struct {
	name     string
	expected string
	err      error
	cfg      map[configura.Variable[string]]string
}

var TestSetOpenFeatureProviderTestCases = []TestSetOpenFeatureProviderTestCase{
	{
		name:     "Default NoopProvider",
		expected: "NoopProvider",
		cfg:      map[configura.Variable[string]]string{},
		err:      nil,
	},
	{
		name:     "NoopProvider without URL",
		expected: "NoopProvider",
		cfg: map[configura.Variable[string]]string{
			SERVER_OPENFEATURE_PROVIDER_NAME: "NoopProvider",
		},
	},
	{
		name:     "Go Feature Flag Provider",
		expected: "GO Feature Flag Provider",
		cfg: map[configura.Variable[string]]string{
			SERVER_OPENFEATURE_PROVIDER_NAME: "go-feature-flag",
			SERVER_OPENFEATURE_PROVIDER_URL:  "http://custom-provider.example.com",
		},
		err: nil,
	},
	{
		name: "Provider name given, missing url",
		err:  ErrOpenFeatureProviderURLNotSet,
		cfg: map[configura.Variable[string]]string{
			SERVER_OPENFEATURE_PROVIDER_NAME: "go-feature-flag",
		},
	},
	{
		name:     "Provider URL given, missing name",
		expected: "NoopProvider",
		cfg: map[configura.Variable[string]]string{
			SERVER_OPENFEATURE_PROVIDER_URL: "http://custom-provider.example.com",
		},
	},
	{
		name: "Provider URL invalid",
		err:  ErrInvalidOpenFeatureProviderURL,
		cfg: map[configura.Variable[string]]string{
			SERVER_OPENFEATURE_PROVIDER_NAME: "go-feature-flag",
			SERVER_OPENFEATURE_PROVIDER_URL:  "http:/i\nvalid-url",
		},
	},
	{
		name: "Unsupported OpenFeature Provider",
		err:  ErrUnsupportedOpenFeatureProvider,
		cfg: map[configura.Variable[string]]string{
			SERVER_OPENFEATURE_PROVIDER_NAME: "unknown-provider-name",
			SERVER_OPENFEATURE_PROVIDER_URL:  "http://custom-provider.example.com",
		},
	},
}

// TestSetOpenFeatureProvider tests the SetOpenFeatureProvider function to ensure it correctly sets the OpenFeature
// provider based on the provided configuration.
func TestSetOpenFeatureProvider(t *testing.T) {
	for _, tc := range TestSetOpenFeatureProviderTestCases {
		t.Run(tc.name, func(t *testing.T) {
			emptyCfg := configura.NewConfigImpl()
			err := configura.WriteConfiguration(emptyCfg, tc.cfg)
			require.NoError(t, err, "Failed to write free port to configuration")

			t.Logf("Running test case: %s ", tc.name)
			err = setOpenFeatureProvider(emptyCfg)
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
				return
			}

			assert.NoError(t, err)
			metadata := openfeature.NamedProviderMetadata("")
			assert.Equal(t, tc.expected, metadata.Name)

			openfeatureClient := openfeature.NewClient("test")
			assert.Equal(t, "test", openfeatureClient.Metadata().Domain())
		})
	}
}
