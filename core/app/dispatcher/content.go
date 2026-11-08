package dispatcher

import (
	"context"
	"maps"

	"github.com/xtls/xray-core/common/session"
)

// contextWithDispatchContent gives each logical stream its own mutable metadata.
// Multiplexed inbounds (e.g. AnyTLS) reuse their parent context across Dispatch
// calls. That inherited Content is an input template: sniffing and routing must
// not write back to it, even when sniffing is disabled (forcedOutboundTag is
// consumed by routedDispatch). A shallow copy alone would still share the map.
func contextWithDispatchContent(ctx context.Context) (context.Context, *session.Content) {
	content := new(session.Content)
	if parent := session.ContentFromContext(ctx); parent != nil {
		*content = *parent
		content.Attributes = maps.Clone(parent.Attributes)
	}
	return session.ContextWithContent(ctx, content), content
}
