<?php
/**
 * Plugin Name: FOD API Bridge
 * Description: Exposes ACF Site Settings data via REST API for Go site migration.
 * Version: 1.0
 *
 * Upload to: wp-content/mu-plugins/fod-api-bridge.php
 * Remove after migration is complete.
 */

add_action('rest_api_init', function () {
    register_rest_route('fod-bridge/v1', '/site-settings', array(
        'methods'  => 'GET',
        'callback' => 'fod_bridge_get_site_settings',
        'permission_callback' => function () {
            // Simple secret key auth — change this value if you want
            return isset($_GET['key']) && $_GET['key'] === 'fod-migrate-2026';
        },
    ));
});

function fod_bridge_get_site_settings() {
    if (!function_exists('get_field')) {
        return new WP_Error('acf_missing', 'ACF plugin not active', array('status' => 500));
    }

    $leagues = array('mlb', 'aaa', 'aa', 'high_a');
    $data = array();

    // ISBP Balances
    foreach ($leagues as $lg) {
        $data['isbp_' . $lg] = get_field('isbp_' . $lg, 'option') ?: array();
    }

    // MILB Balances
    foreach ($leagues as $lg) {
        $data['milb_' . $lg] = get_field('milb_' . $lg, 'option') ?: array();
    }

    // Luxury Tax Thresholds
    $data['luxury_tax_thresholds'] = get_field('luxury_tax_thresholds', 'option') ?: array();

    // Manual Team Financials (dead cap overrides, etc.)
    $data['manual_team_financials'] = get_field('manual_team_financials', 'option') ?: array();

    // Extension & Restructure Usage Logs
    $data['extension_usage_log'] = get_field('extension_usage_log', 'option') ?: array();
    $data['restructure_usage_log'] = get_field('restructure_usage_log', 'option') ?: array();

    // League Key Dates
    foreach ($leagues as $lg) {
        $data['dates_' . $lg] = get_field('dates_' . $lg, 'option') ?: array();
    }

    // Trade Deadlines & Opening Days
    $data['trade_deadlines'] = get_field('trade_deadlines', 'option') ?: array();
    $data['opening_days'] = get_field('opening_days', 'option') ?: array();

    // Slack Channel Config
    $data['league_slack_channels'] = get_field('league_slack_channels', 'option') ?: array();

    // Page IDs (for reference)
    $page_fields = array(
        'trade_page', 'free_agent_page', 'view_pending_trades_page',
        'roster_page', 'waiver_wire_page', 'league_rosters_page',
        'registration_page',
    );
    $data['page_ids'] = array();
    foreach ($page_fields as $f) {
        $data['page_ids'][$f] = get_field($f, 'option') ?: '';
    }

    return rest_ensure_response($data);
}
