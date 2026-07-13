mod agents_md;
mod database;
mod settings;
mod skills;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            settings::get_app_settings,
            settings::get_system_fonts,
            settings::set_font_size,
            settings::set_font_family,
            settings::set_document_preview_layout,
            agents_md::get_agents_md_dashboard,
            agents_md::create_agents_md_fragment,
            agents_md::update_agents_md_fragment,
            agents_md::delete_agents_md_fragment,
            agents_md::set_agents_md_fragment_enabled,
            agents_md::reorder_agents_md_fragments,
            agents_md::open_agents_md_folder,
            skills::get_skills_dashboard,
            skills::install_skill_source,
            skills::update_skill_source,
            skills::update_all_skill_sources,
            skills::set_global_skill_enabled,
            skills::get_skill_detail,
            skills::get_skill_document,
            skills::open_skill_document,
            skills::open_skill_folder,
            skills::open_global_skills_folder,
            skills::get_remove_source_impact,
            skills::remove_skill_source,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
