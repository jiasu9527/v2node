package node

import (
	"context"

	log "github.com/sirupsen/logrus"
	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/wyx2685/v2node/core"
)

func isExternalNode(node *panel.NodeInfo) bool {
	if node == nil {
		return false
	}
	if panel.IsExternalProtocol(node.Type) {
		return true
	}
	if node.Common != nil {
		return node.Common.ExternalProtocol || panel.IsExternalProtocol(node.Common.Protocol)
	}
	return false
}

func (c *Controller) startExternalProtocol(node *panel.NodeInfo) error {
	process, err := core.NewExternalProcess(node, c.userList)
	if err != nil {
		return err
	}
	c.info = node
	log.WithFields(log.Fields{
		"tag":         c.tag,
		"protocol":    node.Type,
		"config_path": process.ConfigPath,
	}).Info("Rendered external protocol config without Xray inbound")
	c.startTasks(node)
	return nil
}

func (c *Controller) reportExternalUnsupportedTrafficTask(_ context.Context) error {
	log.WithField("tag", c.tag).Debug("Skip user traffic report: external protocol traffic_mode=unsupported")
	return nil
}
