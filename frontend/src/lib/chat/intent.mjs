const supported = new Set([
 'generate_image','generate_character_reference','generate_character_references','generate_script','generate_scenes','generate_shots','storyboard','render_scene',
 'render_shot','retry_render','generate_assets','refine_scene','refine_shot',
 'delete_scene','style_ingest','plan_assets','open_editor','update_character','finish_video',
 'render_scenes','check_video','stop_waiting','switch_video_service','clear_failed_videos','fit_script','repair_clip_speech','ai_edit'
]);
/** Normalize both current replies and cards restored from older browser sessions. */
export function normalizeChatIntent(intent) {
 let action=intent.action;
 if(action==='create_script'||action==='add_scene')action='generate_script';
 return {...intent,action:supported.has(action)?action:'unknown'};
}
