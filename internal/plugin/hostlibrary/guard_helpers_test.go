package hostlibrary

// allowAllGuard binds a guard that grants every permission and applies no
// rate limit to the plugin, for tests that exercise a module's own logic.
func allowAllGuard(pluginID uint64) *PluginGuard {
	return NewGuard(stubPermissionChecker{allowed: true}).For(pluginID)
}
