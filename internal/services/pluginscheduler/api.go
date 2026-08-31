package pluginscheduler

import (
	"context"
	"regexp"

	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
)

var taskNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// AddTask validates and upserts a task registration. It is invoked from
// plugin host functions, which may run while the plugin manager lock is held
// (guest Initialize/Shutdown), so this path must never call into
// PluginProvider — the handler-export check happens at fire time instead.
func (s *Service) AddTask(ctx context.Context, task domain.PluginScheduledTask) error {
	if err := s.validateTask(&task); err != nil {
		return err
	}

	// The cap check, persistence and arming stay under one lock so concurrent
	// registrations cannot both pass the per-plugin limit. Holding s.mu across
	// the DB write is deadlock-free (nothing here touches the plugin manager)
	// and only briefly delays the scheduler loop.
	s.mu.Lock()
	defer s.mu.Unlock()

	key := taskKey{pluginID: task.PluginID, name: task.Name}

	if _, exists := s.tasks[key]; !exists {
		count := 0
		for k := range s.tasks {
			if k.pluginID == task.PluginID {
				count++
			}
		}

		if count >= s.opts.MaxTasksPerPlugin {
			return errors.Wrapf(ErrTaskLimitReached, "at most %d tasks per plugin", s.opts.MaxTasksPerPlugin)
		}
	}

	if err := s.repo.Upsert(ctx, &task); err != nil {
		return errors.WithMessage(err, "failed to persist scheduled task")
	}

	if st, ok := s.tasks[key]; ok {
		st.task = task
		st.nextAt = nextSlot(s.clock.Now(), task.Interval)
	} else {
		s.tasks[key] = &taskState{task: task, nextAt: nextSlot(s.clock.Now(), task.Interval)}
	}

	s.notify()

	return nil
}

func (s *Service) validateTask(task *domain.PluginScheduledTask) error {
	if task.PluginID == 0 {
		return errors.Wrap(ErrInvalidTask, "plugin id is not known")
	}

	if !taskNameRegex.MatchString(task.Name) {
		return errors.Wrap(ErrInvalidTask,
			"task name must match ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$")
	}

	if task.Interval < s.opts.MinInterval {
		return errors.Wrapf(ErrInvalidTask,
			"interval %s is below the minimum %s", task.Interval, s.opts.MinInterval)
	}

	if task.ErrorPolicy == "" {
		task.ErrorPolicy = domain.PluginScheduledTaskErrorPolicyIgnore
	}

	if task.ErrorPolicy != domain.PluginScheduledTaskErrorPolicyIgnore &&
		task.ErrorPolicy != domain.PluginScheduledTaskErrorPolicyRetry {
		return errors.Wrapf(ErrInvalidTask, "unknown error policy %q", task.ErrorPolicy)
	}

	if err := s.validateRetryPolicy(task); err != nil {
		return err
	}

	if task.Timeout < 0 {
		return errors.Wrap(ErrInvalidTask, "timeout must not be negative")
	}
	task.Timeout = min(task.Timeout, s.opts.MaxCallTimeout)

	return nil
}

func (s *Service) validateRetryPolicy(task *domain.PluginScheduledTask) error {
	if task.MaxRetries > uint(s.opts.MaxRetries) {
		return errors.Wrapf(ErrInvalidTask,
			"max retries %d exceeds the limit %d", task.MaxRetries, s.opts.MaxRetries)
	}

	if task.RetryDelay < 0 || task.RetryDelay > s.opts.MaxRetryDelay {
		return errors.Wrapf(ErrInvalidTask,
			"retry delay must be within 0..%s", s.opts.MaxRetryDelay)
	}

	if task.MaxJitter < 0 || task.MaxJitter > s.opts.MaxJitter {
		return errors.Wrapf(ErrInvalidTask,
			"retry jitter must be within 0..%s", s.opts.MaxJitter)
	}

	return nil
}

func (s *Service) RemoveTask(ctx context.Context, pluginID domain.Uint64ID, name string) error {
	if err := s.repo.Delete(ctx, pluginID, name); err != nil {
		return errors.WithMessage(err, "failed to delete scheduled task")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tasks, taskKey{pluginID: pluginID, name: name})
	s.notify()

	return nil
}

// ListTasks reads from the database rather than the local registry so the
// answer is consistent across panel instances.
func (s *Service) ListTasks(ctx context.Context, pluginID domain.Uint64ID) ([]domain.PluginScheduledTask, error) {
	tasks, err := s.repo.FindByPlugin(ctx, pluginID)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to fetch scheduled tasks")
	}

	return tasks, nil
}

// RemovePluginTasks drops every task of the plugin; called on uninstall.
// Other instances pick the removal up on their next refresh.
func (s *Service) RemovePluginTasks(ctx context.Context, pluginID domain.Uint64ID) (int, error) {
	deleted, err := s.repo.DeleteByPlugin(ctx, pluginID)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to delete scheduled tasks")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for key := range s.tasks {
		if key.pluginID == pluginID {
			delete(s.tasks, key)
		}
	}
	s.notify()

	return deleted, nil
}
