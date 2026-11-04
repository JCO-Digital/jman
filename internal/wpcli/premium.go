package wpcli

// premiumUpdatesPHP runs through wp-cli's --exec, before WordPress loads,
// and registers update checks that premium plugins only hook in admin
// requests, so `wp plugin list` and `wp plugin update` see their updates
// (#61). It touches nothing else: other plugins still load in WP-CLI's
// normal, non-admin context, unlike defining WP_ADMIN.
//
//   - Yoast SEO's add-ons (Premium, Local, News, Video, WooCommerce SEO)
//     update through WPSEO_Addon_Manager in Yoast SEO itself, which
//     registers its pre_set_site_transient_update_plugins filter only
//     when is_admin() (wp-seo-main.php). The check itself has no admin
//     dependency. Priority 20 runs after Yoast loads (plugins_loaded 14).
//
// FileBird Pro, which checks current_user_can('manage_options') instead,
// is handled by running as an administrator (--user).
//
// It must stay a no-op wherever a plugin is missing, renamed or already
// hooked, and must not fail on old WP-CLI versions without add_wp_hook.
const premiumUpdatesPHP = `if (method_exists('WP_CLI', 'add_wp_hook')) { WP_CLI::add_wp_hook('plugins_loaded', function () { ` +
	`if (!is_admin() && class_exists('WPSEO_Addon_Manager') && method_exists('WPSEO_Addon_Manager', 'check_for_updates')) { ` +
	`add_filter('pre_set_site_transient_update_plugins', [new WPSEO_Addon_Manager(), 'check_for_updates']); } }, 20); }`

// PremiumUpdatesArg is the wp-cli global argument that makes premium
// plugins report and install their updates; see premiumUpdatesPHP. It only
// matters with plugins loaded.
func PremiumUpdatesArg() string {
	return "--exec=" + premiumUpdatesPHP
}
