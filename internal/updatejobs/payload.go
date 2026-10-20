package updatejobs

import "github.com/JCO-Digital/jman/internal/models"

// checkPayload reports whether a job's payload decodes for its kind. run
// checks it before running the job, so the accessors below can ignore
// decode errors.
func checkPayload(job models.UpdateJob) error {
	var err error
	switch {
	case job.Kind == models.UpdateJobKindCore:
		_, err = models.DecodePayload[models.CorePayload](job)
	case job.Kind == models.UpdateJobKindInstall:
		_, err = models.DecodePayload[models.InstallPayload](job)
	case job.Kind == models.UpdateJobKindUserCreate:
		_, err = models.DecodePayload[models.UserCreatePayload](job)
	case job.Kind == models.UpdateJobKindUserDelete:
		_, err = models.DecodePayload[models.UserDeletePayload](job)
	default:
		_, err = models.DecodePayload[models.PluginsPayload](job)
	}
	return err
}

func jobPluginsPayload(job models.UpdateJob) models.PluginsPayload {
	p, _ := models.DecodePayload[models.PluginsPayload](job)
	return p
}

// jobPlugins returns the plugins of a plugin update or management job.
func jobPlugins(job models.UpdateJob) []models.PluginUpdateRequest {
	return jobPluginsPayload(job).Plugins
}

func jobCore(job models.UpdateJob) models.CorePayload {
	p, _ := models.DecodePayload[models.CorePayload](job)
	return p
}

func jobInstall(job models.UpdateJob) models.InstallPayload {
	p, _ := models.DecodePayload[models.InstallPayload](job)
	return p
}
