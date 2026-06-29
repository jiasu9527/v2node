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

func (c *Controller) reloadExternalProtocol(node *panel.NodeInfo, users []panel.UserInfo) error {
	if c.externalProcess != nil {
		if err := c.externalProcess.Stop(); err != nil {
			return err
		}
		c.externalProcess = nil
	}
	process, err := core.NewExternalProcess(node, users)
	if err != nil {
		return err
	}
	if err := process.Start(); err != nil {
		return err
	}
	c.externalProcess = process
	return nil
}

func (c *Controller) startExternalProtocol(node *panel.NodeInfo) error {
	if err := c.reloadExternalProtocol(node, c.userList); err != nil {
		return err
	}
	process := c.externalProcess
	c.info = node
	log.WithFields(log.Fields{
		"tag":         c.tag,
		"protocol":    node.Type,
		"config_path": process.ConfigPath,
		"command":     process.Command,
	}).Info("Started external protocol process without Xray inbound")
	c.startTasks(node)
	return nil
}

func (c *Controller) reportExternalUnsupportedTrafficTask(_ context.Context) error {
	log.WithField("tag", c.tag).Debug("Skip user traffic report: external protocol traffic_mode=unsupported")
	return nil
}
