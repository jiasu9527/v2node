package node

import (
	"context"
	"errors"

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
	if c.embeddedProtocolServer != nil {
		if err := c.embeddedProtocolServer.Stop(); err != nil {
			return err
		}
		c.embeddedProtocolServer = nil
	}
	server, err := core.NewEmbeddedProtocolServer(node, users)
	if err != nil {
		return err
	}
	c.externalTrafficCollector = core.NewExternalTrafficCollector(node)
	if err := server.Start(context.Background()); err != nil {
		return err
	}
	c.embeddedProtocolServer = server
	return nil
}

func (c *Controller) startExternalProtocol(node *panel.NodeInfo) error {
	if err := c.reloadExternalProtocol(node, c.userList); err != nil {
		return err
	}
	c.info = node
	log.WithFields(log.Fields{
		"tag":      c.tag,
		"protocol": node.Type,
	}).Info("Started embedded protocol server without Xray inbound")
	c.startTasks(node)
	return nil
}

func (c *Controller) reportExternalUnsupportedTrafficTask(_ context.Context) error {
	log.WithField("tag", c.tag).Debug("Skip user traffic report: external protocol traffic_mode=unsupported")
	return nil
}

func (c *Controller) reportExternalUserTrafficTask(ctx context.Context) error {
	if c.externalTrafficCollector == nil {
		c.externalTrafficCollector = core.NewExternalTrafficCollector(c.info)
	}
	reportmin := 0
	if c.info != nil && c.info.Common != nil && c.info.Common.BaseConfig != nil {
		reportmin = c.info.Common.BaseConfig.NodeReportMinTraffic
	}
	userTraffic, err := c.externalTrafficCollector.CollectTraffic(c.userList, reportmin)
	if err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Collect external user traffic failed")
		return nil
	}
	if len(userTraffic) == 0 {
		return nil
	}
	if err := c.apiClient.ReportUserTraffic(ctx, userTraffic); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Report external user traffic failed")
		return nil
	}
	log.WithField("tag", c.tag).Infof("Report %d external users traffic", len(userTraffic))
	return nil
}

func (c *Controller) reportExternalOnlineUsersTask(ctx context.Context) error {
	if c.externalTrafficCollector == nil {
		c.externalTrafficCollector = core.NewExternalTrafficCollector(c.info)
	}
	devicemin := 0
	if c.info != nil && c.info.Common != nil && c.info.Common.BaseConfig != nil {
		devicemin = c.info.Common.BaseConfig.DeviceOnlineMinTraffic
	}
	onlineUsers, err := c.externalTrafficCollector.CollectOnlineUsers(c.userList, devicemin)
	if err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Debug("Skip external online user report")
		return nil
	}
	data := buildOnlineUserReportData(onlineUsers, nil, false)
	if len(data) == 0 {
		return nil
	}
	if err := c.apiClient.ReportNodeOnlineUsers(ctx, &data); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Report external online users failed")
		return nil
	}
	log.WithField("tag", c.tag).Infof("Report %d external online UIDs", len(data))
	return nil
}

func (c *Controller) reportExternalSensitiveAccessTask(ctx context.Context) error {
	if c.externalTrafficCollector == nil {
		c.externalTrafficCollector = core.NewExternalTrafficCollector(c.info)
	}
	events, err := c.externalTrafficCollector.CollectSensitiveAccess(c.userList)
	if err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Debug("Skip external sensitive access report")
		return nil
	}
	if len(events) == 0 {
		return nil
	}
	if err := c.apiClient.ReportSensitiveAccess(ctx, events); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Report external sensitive access failed")
		return nil
	}
	log.WithField("tag", c.tag).Infof("Report %d external sensitive access events", len(events))
	return nil
}
