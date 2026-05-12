package gateway

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func DomainServerToProto(srv *domain.Server) *proto.Server {
	var createdAt, updatedAt, deletedAt, expires, lastProcessCheck *timestamppb.Timestamp

	if srv.CreatedAt != nil {
		createdAt = timestamppb.New(*srv.CreatedAt)
	}
	if srv.UpdatedAt != nil {
		updatedAt = timestamppb.New(*srv.UpdatedAt)
	}
	if srv.DeletedAt != nil {
		deletedAt = timestamppb.New(*srv.DeletedAt)
	}
	if srv.Expires != nil {
		expires = timestamppb.New(*srv.Expires)
	}
	if srv.LastProcessCheck != nil {
		lastProcessCheck = timestamppb.New(*srv.LastProcessCheck)
	}

	var queryPort, rconPort *int32
	if srv.QueryPort != nil {
		queryPort = new(clampToInt32(*srv.QueryPort))
	}
	if srv.RconPort != nil {
		rconPort = new(clampToInt32(*srv.RconPort))
	}

	var cpuLimit, ramLimit, netLimit *int32
	if srv.CPULimit != nil {
		cpuLimit = new(clampToInt32(*srv.CPULimit))
	}
	if srv.RAMLimit != nil {
		ramLimit = new(clampToInt32(*srv.RAMLimit))
	}
	if srv.NetLimit != nil {
		netLimit = new(clampToInt32(*srv.NetLimit))
	}

	var varsStr *string
	if srv.Vars != nil {
		varsBytes, err := json.Marshal(srv.Vars)
		if err == nil {
			varsStr = new(string(varsBytes))
		}
	}

	return &proto.Server{
		Id:               uint64(srv.ID),
		Uuid:             srv.UUID.String(),
		UuidShort:        srv.UUIDShort,
		Enabled:          srv.Enabled,
		Installed:        domainInstalledStatusToProto(srv.Installed),
		Blocked:          srv.Blocked,
		Name:             srv.Name,
		GameId:           srv.GameID,
		DsId:             uint64(srv.DSID),
		GameModId:        uint64(srv.GameModID),
		Expires:          expires,
		ServerIp:         srv.ServerIP,
		ServerPort:       clampToInt32(srv.ServerPort),
		QueryPort:        queryPort,
		RconPort:         rconPort,
		Rcon:             srv.Rcon,
		Dir:              srv.Dir,
		SuUser:           srv.SuUser,
		CpuLimit:         cpuLimit,
		RamLimit:         ramLimit,
		NetLimit:         netLimit,
		StartCommand:     srv.StartCommand,
		StopCommand:      srv.StopCommand,
		ForceStopCommand: srv.ForceStopCommand,
		RestartCommand:   srv.RestartCommand,
		ProcessActive:    srv.ProcessActive,
		LastProcessCheck: lastProcessCheck,
		Vars:             varsStr,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		DeletedAt:        deletedAt,
		Metadata:         domainMetadataToProto(srv.Metadata),
	}
}

func DomainServerToProtoWithGameMod(
	srv *domain.Server, gameMod *domain.GameMod, nodeOS domain.NodeOS,
) *proto.Server {
	p := DomainServerToProto(srv)

	if gameMod == nil {
		return p
	}

	startCmd := gameMod.StartCmdLinux
	if nodeOS == domain.NodeOSWindows {
		startCmd = gameMod.StartCmdWindows
	}

	if (p.StartCommand == nil || *p.StartCommand == "") && startCmd != nil {
		p.StartCommand = startCmd
	}

	return p
}

func domainInstalledStatusToProto(status domain.ServerInstalledStatus) proto.ServerInstalledStatus {
	switch status {
	case domain.ServerInstalledStatusNotInstalled:
		return proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_NOT_INSTALLED
	case domain.ServerInstalledStatusInstalled:
		return proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLED
	case domain.ServerInstalledStatusInstallationInProg:
		return proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLATION_IN_PROGRESS
	default:
		return proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_NOT_INSTALLED
	}
}

func DomainGameToProto(g *domain.Game) *proto.Game {
	var steamAppIDLinux, steamAppIDWindows *uint32
	if g.SteamAppIDLinux != nil {
		steamAppIDLinux = new(clampToUint32(*g.SteamAppIDLinux))
	}
	if g.SteamAppIDWindows != nil {
		steamAppIDWindows = new(clampToUint32(*g.SteamAppIDWindows))
	}

	return &proto.Game{
		Code:                    g.Code,
		Name:                    g.Name,
		Engine:                  g.Engine,
		EngineVersion:           g.EngineVersion,
		SteamAppIdLinux:         steamAppIDLinux,
		SteamAppIdWindows:       steamAppIDWindows,
		SteamAppSetConfig:       g.SteamAppSetConfig,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		Enabled:                 g.Enabled != 0,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		Metadata:                domainMetadataToProto(g.Metadata),
	}
}

func DomainGameModToProto(gm *domain.GameMod) *proto.GameMod {
	fastRcon := make([]*proto.GameModFastRcon, 0, len(gm.FastRcon))
	for _, fr := range gm.FastRcon {
		fastRcon = append(fastRcon, &proto.GameModFastRcon{
			Info:    fr.Info,
			Command: fr.Command,
		})
	}

	vars := make([]*proto.GameModVar, 0, len(gm.Vars))
	for _, v := range gm.Vars {
		vars = append(vars, &proto.GameModVar{
			Var:      v.Var,
			Default:  string(v.Default),
			Info:     v.Info,
			AdminVar: v.AdminVar,
		})
	}

	return &proto.GameMod{
		Id:                      uint64(gm.ID),
		GameCode:                gm.GameCode,
		Name:                    gm.Name,
		StartCmdLinux:           gm.StartCmdLinux,
		StartCmdWindows:         gm.StartCmdWindows,
		KickCmd:                 gm.KickCmd,
		BanCmd:                  gm.BanCmd,
		FastRcon:                fastRcon,
		Vars:                    vars,
		RemoteRepositoryLinux:   gm.RemoteRepositoryLinux,
		RemoteRepositoryWindows: gm.RemoteRepositoryWindows,
		LocalRepositoryLinux:    gm.LocalRepositoryLinux,
		LocalRepositoryWindows:  gm.LocalRepositoryWindows,
		ChnameCmd:               gm.ChnameCmd,
		SrestartCmd:             gm.SrestartCmd,
		ChmapCmd:                gm.ChmapCmd,
		SendmsgCmd:              gm.SendmsgCmd,
		PasswdCmd:               gm.PasswdCmd,
		Metadata:                domainMetadataToProto(gm.Metadata),
	}
}

func domainMetadataToProto(metadata domain.Metadata) map[string]*anypb.Any {
	if metadata == nil {
		return nil
	}

	result := make(map[string]*anypb.Any, len(metadata))
	for k, v := range metadata {
		anyVal, err := anypb.New(wrapperspb.String(fmt.Sprint(v)))
		if err != nil {
			continue
		}
		result[k] = anyVal
	}

	return result
}

func DomainDaemonTaskToProto(task *domain.DaemonTask) *proto.DaemonTask {
	var createdAt, updatedAt *timestamppb.Timestamp
	if task.CreatedAt != nil {
		createdAt = timestamppb.New(*task.CreatedAt)
	}
	if task.UpdatedAt != nil {
		updatedAt = timestamppb.New(*task.UpdatedAt)
	}

	var serverID *uint64
	if task.ServerID != nil {
		serverID = new(uint64(*task.ServerID))
	}

	var runAfterID *uint64
	if task.RunAftID != nil {
		runAfterID = new(uint64(*task.RunAftID))
	}

	return &proto.DaemonTask{
		Id:         uint64(task.ID),
		RunAfterId: runAfterID,
		NodeId:     uint64(task.DedicatedServerID),
		ServerId:   serverID,
		TaskType:   domainTaskTypeToProto(task.Task),
		Data:       task.Data,
		Cmd:        task.Cmd,
		Output:     task.Output,
		Status:     domainTaskStatusToProto(task.Status),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
}

func domainTaskTypeToProto(taskType domain.DaemonTaskType) proto.DaemonTaskType {
	switch taskType {
	case domain.DaemonTaskTypeServerStart:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START
	case domain.DaemonTaskTypeServerStop:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_STOP
	case domain.DaemonTaskTypeServerRestart:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_RESTART
	case domain.DaemonTaskTypeServerUpdate:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_UPDATE
	case domain.DaemonTaskTypeServerInstall:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL
	case domain.DaemonTaskTypeServerDelete:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_DELETE
	case domain.DaemonTaskTypeServerMove:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_MOVE
	case domain.DaemonTaskTypeCmdExec:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC
	default:
		return proto.DaemonTaskType_DAEMON_TASK_TYPE_UNSPECIFIED
	}
}

func domainTaskStatusToProto(status domain.DaemonTaskStatus) proto.DaemonTaskStatus {
	switch status {
	case domain.DaemonTaskStatusWaiting:
		return proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING
	case domain.DaemonTaskStatusWorking:
		return proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING
	case domain.DaemonTaskStatusError:
		return proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR
	case domain.DaemonTaskStatusSuccess:
		return proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS
	case domain.DaemonTaskStatusCanceled:
		return proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED
	default:
		return proto.DaemonTaskStatus_DAEMON_TASK_STATUS_UNSPECIFIED
	}
}

func ProtoTaskStatusToDomain(status proto.DaemonTaskStatus) domain.DaemonTaskStatus {
	switch status {
	case proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING:
		return domain.DaemonTaskStatusWaiting
	case proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING:
		return domain.DaemonTaskStatusWorking
	case proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR:
		return domain.DaemonTaskStatusError
	case proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS:
		return domain.DaemonTaskStatusSuccess
	case proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED:
		return domain.DaemonTaskStatusCanceled
	default:
		return domain.DaemonTaskStatusWaiting
	}
}

func DomainServerSettingsToProto(settings []domain.ServerSetting) []*proto.ServerSetting {
	result := make([]*proto.ServerSetting, 0, len(settings))
	for _, s := range settings {
		valueStr, _ := s.Value.String()
		result = append(result, &proto.ServerSetting{
			Id:       uint64(s.ID),
			ServerId: uint64(s.ServerID),
			Name:     s.Name,
			Value:    valueStr,
		})
	}

	return result
}

func clampToInt32(v int) int32 {
	return int32(min(v, math.MaxInt32)) //nolint:gosec // value clamped to int32 range
}

func clampToUint32(v uint) uint32 {
	return uint32(min(v, math.MaxUint32)) //nolint:gosec // value clamped to uint32 range
}

func DomainServerTaskToProto(task *domain.ServerTask) *proto.ServerTask {
	var nodeID uint64
	if task.NodeID != nil {
		nodeID = uint64(*task.NodeID)
	}

	var name, timezone, payload string
	if task.Name != nil {
		name = *task.Name
	}
	if task.Timezone != nil {
		timezone = *task.Timezone
	}
	if task.Payload != nil {
		payload = *task.Payload
	}

	var updatedAt *timestamppb.Timestamp
	if task.UpdatedAt != nil {
		updatedAt = timestamppb.New(*task.UpdatedAt)
	}

	return &proto.ServerTask{
		Id:            uint64(task.ID),
		ServerId:      uint64(task.ServerID),
		NodeId:        nodeID,
		Command:       domainServerTaskCommandToProto(task.Command),
		ExecuteDate:   timestamppb.New(task.ExecuteDate),
		RepeatPeriod:  durationpb.New(task.RepeatPeriod),
		RepeatCount:   uint32(task.Repeat),
		Counter:       clampToUint32(task.Counter),
		Payload:       payload,
		Version:       task.Version,
		OverlapPolicy: domainServerTaskOverlapPolicyToProto(task.OverlapPolicy),
		CatchupPolicy: domainServerTaskCatchupPolicyToProto(task.CatchupPolicy),
		Enabled:       task.Enabled,
		Name:          name,
		Timezone:      timezone,
		UpdatedAt:     updatedAt,
	}
}

func DomainServerTasksToProtoList(tasks []domain.ServerTask) []*proto.ServerTask {
	out := make([]*proto.ServerTask, 0, len(tasks))
	for i := range tasks {
		out = append(out, DomainServerTaskToProto(&tasks[i]))
	}

	return out
}

func domainServerTaskCommandToProto(c domain.ServerTaskCommand) proto.ServerTaskCommand {
	switch c {
	case domain.ServerTaskCommandStart:
		return proto.ServerTaskCommand_SERVER_TASK_COMMAND_START
	case domain.ServerTaskCommandStop:
		return proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP
	case domain.ServerTaskCommandRestart:
		return proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART
	case domain.ServerTaskCommandUpdate:
		return proto.ServerTaskCommand_SERVER_TASK_COMMAND_UPDATE
	case domain.ServerTaskCommandReinstall:
		return proto.ServerTaskCommand_SERVER_TASK_COMMAND_REINSTALL
	default:
		return proto.ServerTaskCommand_SERVER_TASK_COMMAND_UNSPECIFIED
	}
}

func ProtoServerTaskCommandToDomain(c proto.ServerTaskCommand) domain.ServerTaskCommand {
	switch c {
	case proto.ServerTaskCommand_SERVER_TASK_COMMAND_START:
		return domain.ServerTaskCommandStart
	case proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP:
		return domain.ServerTaskCommandStop
	case proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART:
		return domain.ServerTaskCommandRestart
	case proto.ServerTaskCommand_SERVER_TASK_COMMAND_UPDATE:
		return domain.ServerTaskCommandUpdate
	case proto.ServerTaskCommand_SERVER_TASK_COMMAND_REINSTALL:
		return domain.ServerTaskCommandReinstall
	default:
		return ""
	}
}

func domainServerTaskOverlapPolicyToProto(p domain.ServerTaskOverlapPolicy) proto.ServerTaskOverlapPolicy {
	switch p {
	case domain.ServerTaskOverlapPolicySkip:
		return proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_SKIP
	case domain.ServerTaskOverlapPolicyQueue:
		return proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_QUEUE
	default:
		return proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_SKIP
	}
}

func domainServerTaskCatchupPolicyToProto(p domain.ServerTaskCatchupPolicy) proto.ServerTaskCatchupPolicy {
	switch p {
	case domain.ServerTaskCatchupPolicySkip:
		return proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_SKIP
	case domain.ServerTaskCatchupPolicyRunOnce:
		return proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_RUN_ONCE
	default:
		return proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_SKIP
	}
}

func ProtoServerTaskExecutionStatusToDomain(s proto.ServerTaskExecutionStatus) domain.ServerTaskExecutionStatus {
	switch s {
	case proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_RUNNING:
		return domain.ServerTaskExecutionStatusRunning
	case proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS:
		return domain.ServerTaskExecutionStatusSuccess
	case proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_FAILED:
		return domain.ServerTaskExecutionStatusFailed
	case proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_CANCELED:
		return domain.ServerTaskExecutionStatusCanceled
	case proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SKIPPED:
		return domain.ServerTaskExecutionStatusSkipped
	case proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_TIMED_OUT:
		return domain.ServerTaskExecutionStatusTimedOut
	default:
		return ""
	}
}
