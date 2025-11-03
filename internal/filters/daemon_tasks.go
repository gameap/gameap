package filters

import "github.com/gameap/gameap/internal/domain"

type FindDaemonTask struct {
	IDs                []uint
	DedicatedServerIDs []uint
	ServerIDs          []*uint
	Tasks              []domain.DaemonTaskType
	Statuses           []domain.DaemonTaskStatus
}

func FindDaemonTaskByIDs(ids []uint) *FindDaemonTask {
	return &FindDaemonTask{
		IDs: ids,
	}
}

func FindDaemonTaskByDedicatedServerIDs(dedicatedServerIDs []uint) *FindDaemonTask {
	return &FindDaemonTask{
		DedicatedServerIDs: dedicatedServerIDs,
	}
}

func FindDaemonTaskByServerIDs(serverIDs []*uint) *FindDaemonTask {
	return &FindDaemonTask{
		ServerIDs: serverIDs,
	}
}

func FindDaemonTaskByTasks(tasks []domain.DaemonTaskType) *FindDaemonTask {
	return &FindDaemonTask{
		Tasks: tasks,
	}
}

func FindDaemonTaskByStatuses(statuses []domain.DaemonTaskStatus) *FindDaemonTask {
	return &FindDaemonTask{
		Statuses: statuses,
	}
}

func FindWaitingDaemonTasks() *FindDaemonTask {
	return &FindDaemonTask{
		Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
	}
}

func FindWorkingDaemonTasks() *FindDaemonTask {
	return &FindDaemonTask{
		Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWorking},
	}
}

func FindActiveDaemonTasks() *FindDaemonTask {
	return &FindDaemonTask{
		Statuses: []domain.DaemonTaskStatus{
			domain.DaemonTaskStatusWaiting,
			domain.DaemonTaskStatusWorking,
		},
	}
}
