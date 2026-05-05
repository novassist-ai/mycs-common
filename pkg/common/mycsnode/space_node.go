package mycsnode

import (
	"context"

	"github.com/novassist/mycs-common/pkg/goutils/rest"
)

// SpaceNode is implemented by cloud space node values used with ApiClient.
type SpaceNode interface {
	Key() string

	GetSpaceID() string
	GetSpaceName() string
	GetPublicKey() string

	GetRecipe() string
	GetIaaS() string
	GetRegion() string
	GetVersion() string

	GetStatus() string
	GetLastSeen() uint64

	IsRunning() bool

	IsSpaceOwned() bool
	HasAdminAccess() bool
	CanUseAsEgressNode() bool

	GetApiCARoot() string
	GetEndpoint() (string, error)
	RestApiClient(ctx context.Context) (*rest.RestApiClient, error)

	CreateDeviceConnectKeyPair() (string, string, error)
}
