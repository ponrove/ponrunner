package ponrunner_test

import (
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner"
	"github.com/stretchr/testify/assert"
)

type TestSetOpenFeatureProviderTestCase struct {
	name     string
	expected string
	err      error
	cfg      configura.Config
}

var TestSetOpenFeatureProviderTestCases = []TestSetOpenFeatureProviderTestCase{
	{
		name:     "Default NoopProvider",
		expected: "NoopProvider",
		cfg:      configura.ConfigImpl{},
		err:      nil,
	},
	{
		name:     "NoopProvider without URL",
		expected: "NoopProvider",
		cfg: &configura.ConfigImpl{
			RegString: map[configura.Variable[string]]string{
				ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME: "NoopProvider",
			},
		},
	},
	{
		name:     "Go Feature Flag Provider",
		expected: "GO Feature Flag Provider",
		cfg: &configura.ConfigImpl{
			RegString: map[configura.Variable[string]]string{
				ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME: "go-feature-flag",
				ponrunner.SERVER_OPENFEATURE_PROVIDER_URL:  "http://custom-provider.example.com",
			},
		},
		err: nil,
	},
	{
		name: "Provider name given, missing url",
		err:  ponrunner.ErrOpenFeatureProviderURLNotSet,
		cfg: &configura.ConfigImpl{
			RegString: map[configura.Variable[string]]string{
				ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME: "go-feature-flag",
			},
		},
	},
	{
		name:     "Provider URL given, missing name",
		expected: "NoopProvider",
		cfg: &configura.ConfigImpl{
			RegString: map[configura.Variable[string]]string{
				ponrunner.SERVER_OPENFEATURE_PROVIDER_URL: "http://custom-provider.example.com",
			},
		},
	},
	{
		name: "Provider URL invalid",
		err:  ponrunner.ErrInvalidOpenFeatureProviderURL,
		cfg: &configura.ConfigImpl{
			RegString: map[configura.Variable[string]]string{
				ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME: "go-feature-flag",
				ponrunner.SERVER_OPENFEATURE_PROVIDER_URL:  "http:/i\nvalid-url",
			},
		},
	},
	{
		name: "Unsupported OpenFeature Provider",
		err:  ponrunner.ErrUnsupportedOpenFeatureProvider,
		cfg: &configura.ConfigImpl{
			RegString: map[configura.Variable[string]]string{
				ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME: "unknown-provider-name",
				ponrunner.SERVER_OPENFEATURE_PROVIDER_URL:  "http://custom-provider.example.com",
			},
		},
	},
}

// TestSetOpenFeatureProvider tests the SetOpenFeatureProvider function to ensure it correctly sets the OpenFeature
// provider based on the provided configuration.
func TestSetOpenFeatureProvider(t *testing.T) {
	for _, tc := range TestSetOpenFeatureProviderTestCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ponrunner.SetOpenFeatureProvider(tc.cfg)
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
