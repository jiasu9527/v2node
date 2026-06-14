package node

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	panel "github.com/wyx2685/v2node/api/v2board"
)

func (c *Controller) reportUserTrafficTask(ctx context.Context) (err error) {
	var reportmin = 0
	if c.info.Common.BaseConfig != nil {
		reportmin = c.info.Common.BaseConfig.NodeReportMinTraffic
	}
	userTraffic, _ := c.server.GetUserTrafficSlice(c.tag, reportmin)
	if len(userTraffic) == 0 {
		return nil
	}
	err = c.apiClient.ReportUserTraffic(ctx, userTraffic)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Report user traffic failed")
	} else {
		log.WithField("tag", c.tag).Infof("Report %d users traffic", len(userTraffic))
		//log.WithField("tag", c.tag).Debugf("User traffic: %+v", userTraffic)
	}
	return nil
}

func (c *Controller) reportOnlineUsersTask(ctx context.Context) (err error) {
	var devicemin = 0
	if c.info.Common.BaseConfig != nil {
		devicemin = c.info.Common.BaseConfig.DeviceOnlineMinTraffic
	}

	var activeUIDs map[int]struct{}
	if devicemin > 0 {
		activeTraffic, trafficErr := c.server.PeekUserTrafficSlice(c.tag, devicemin)
		if trafficErr != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": trafficErr,
			}).Info("Peek user traffic failed")
		} else {
			activeUIDs = buildActiveUIDSet(activeTraffic)
		}
	}

	onlineDevice, err := c.limiter.GetOnlineDevice()
	if err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Get online users failed")
		return nil
	}
	if len(*onlineDevice) == 0 {
		return nil
	}

	data := buildOnlineUserReportData(*onlineDevice, activeUIDs, devicemin > 0)
	if len(data) == 0 {
		log.WithField("tag", c.tag).Infof("Total %d online users, 0 reported", len(*onlineDevice))
		return nil
	}

	if err = c.apiClient.ReportNodeOnlineUsers(ctx, &data); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Report online users failed")
		return nil
	}

	log.WithField("tag", c.tag).Infof("Total %d online users, %d UIDs reported", len(*onlineDevice), len(data))
	return nil
}

func (c *Controller) reportSensitiveAccessTask(ctx context.Context) (err error) {
	if c.info == nil || c.info.Common == nil || c.info.Common.SensitiveAudit == nil || !c.info.Common.SensitiveAudit.Enable {
		return nil
	}
	events := c.server.GetSensitiveAccessSlice(c.tag)
	if len(events) == 0 {
		return nil
	}
	if err = c.apiClient.ReportSensitiveAccess(ctx, events); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("Report sensitive access failed")
		return nil
	}
	log.WithField("tag", c.tag).Infof("Report %d sensitive access events", len(events))
	return nil
}

func buildActiveUIDSet(userTraffic []panel.UserTraffic) map[int]struct{} {
	if len(userTraffic) == 0 {
		return nil
	}

	activeUIDs := make(map[int]struct{}, len(userTraffic))
	for _, traffic := range userTraffic {
		activeUIDs[traffic.UID] = struct{}{}
	}
	return activeUIDs
}

func buildOnlineUserReportData(onlineDevices []panel.OnlineUser, activeUIDs map[int]struct{}, filterByActiveUIDs bool) map[int][]string {
	if len(onlineDevices) == 0 {
		return nil
	}

	data := make(map[int][]string)
	for _, onlineUser := range onlineDevices {
		if filterByActiveUIDs {
			if _, ok := activeUIDs[onlineUser.UID]; !ok {
				continue
			}
		}
		data[onlineUser.UID] = append(data[onlineUser.UID], onlineUser.IP)
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

func compareUserList(old, new []panel.UserInfo) (deleted, added, modified []panel.UserInfo) {
	oldMap := make(map[string]panel.UserInfo, len(old))
	for _, u := range old {
		oldMap[u.Uuid] = u
	}

	for _, u := range new {
		if o, ok := oldMap[u.Uuid]; !ok {
			added = append(added, u)
		} else {
			if o.SpeedLimit != u.SpeedLimit || o.DeviceLimit != u.DeviceLimit {
				modified = append(modified, u)
			}
			delete(oldMap, u.Uuid)
		}
	}

	for _, o := range oldMap {
		deleted = append(deleted, o)
	}

	return deleted, added, modified
}
