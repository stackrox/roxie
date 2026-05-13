package deployer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/stackrox/roxie/internal/dockerauth"
	v1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	openshiftConfigNamespace      = "openshift-config"
	openshiftGlobalPullSecretName = "pull-secret"
	dockerConfigJsonKey           = ".dockerconfigjson"
	registryForDownstreamImages   = "quay.io/rhacs-eng"
)

var (
	namespacedGlobalPullSecret = openshiftConfigNamespace + "/" + openshiftGlobalPullSecretName
)

// dockerConfigJSON represents the structure of a .dockerconfigjson secret value.
type dockerConfigJSON struct {
	Auths map[string]dockerauth.AuthEntry `json:"auths"`
}

// InjectGlobalOpenShiftPullSecret adds registry credentials to the OpenShift global pull secret.
func (d *Deployer) InjectGlobalOpenShiftPullSecret(ctx context.Context) error {
	// Retry on Conflict, AlreadyExists, and NotFound to handle TOCTOU races between
	// the Get and subsequent Create/Update (e.g., secret deleted after Get -> Update
	// returns NotFound; secret created by another caller after Get -> Create returns
	// AlreadyExists).
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return k8sapierrors.IsConflict(err) || k8sapierrors.IsAlreadyExists(err) || k8sapierrors.IsNotFound(err)
	}, func() error {
		return d.injectGlobalOpenShiftPullSecretOnce(ctx)
	})
}

func (d *Deployer) injectGlobalOpenShiftPullSecretOnce(ctx context.Context) error {
	if d.dockerCreds == nil {
		return errors.New("no pull secrets found")
	}
	credentials := *d.dockerCreds

	if d.k8sClient == nil {
		return errors.New("k8s client not initialized")
	}

	secrets := d.k8sClient.CoreV1().Secrets(openshiftConfigNamespace)
	secret, err := secrets.Get(ctx, openshiftGlobalPullSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8sapierrors.IsNotFound(err) {
			return fmt.Errorf("retrieving secret %s: %w", namespacedGlobalPullSecret, err)
		}
		secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      openshiftGlobalPullSecretName,
				Namespace: openshiftConfigNamespace,
			},
			Type: v1.SecretTypeDockerConfigJson,
		}
	}

	modified, err := injectRegistryCredentialsIntoSecret(credentials, secret)
	if err != nil {
		return fmt.Errorf("injecting registry credentials into Kubernetes secret: %w", err)
	}
	if !modified {
		d.logger.Dimf("Global pull secret %s already contains entry for %s, skipping", namespacedGlobalPullSecret, registryForDownstreamImages)
		return nil
	}

	if secret.ResourceVersion == "" {
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating secret %s: %w", namespacedGlobalPullSecret, err)
		}
	} else {
		if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating secret %s: %w", namespacedGlobalPullSecret, err)
		}
	}

	d.logger.Successf("Injected pull secret for %s into %s", registryForDownstreamImages, namespacedGlobalPullSecret)
	return nil
}

// injectRegistryCredentialsIntoSecret mutates the secret in place, returning true if it was modified.
func injectRegistryCredentialsIntoSecret(credentials dockerauth.Credentials, secret *v1.Secret) (bool, error) {
	var cfg dockerConfigJSON
	if data, ok := secret.Data[dockerConfigJsonKey]; ok {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return false, fmt.Errorf("unmarshaling %q in %s: %w", dockerConfigJsonKey, namespacedGlobalPullSecret, err)
		}
	}
	if cfg.Auths == nil {
		cfg.Auths = make(map[string]dockerauth.AuthEntry)
	}

	if _, ok := cfg.Auths[registryForDownstreamImages]; ok {
		return false, nil
	}

	cfg.Auths[registryForDownstreamImages] = dockerauth.AuthEntry{
		Auth: base64.StdEncoding.EncodeToString([]byte(credentials.Username + ":" + credentials.Password)),
	}

	updated, err := json.Marshal(cfg)
	if err != nil {
		return false, fmt.Errorf("marshaling updated docker config: %w", err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[dockerConfigJsonKey] = updated

	return true, nil
}
