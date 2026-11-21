/** One place that knows how to open the video editor (Studio → Edit video). */
const edit = (query: string) => `/?view=edit&${query}`;
export const editOutputHref = (outputId: string) => edit(`source=${encodeURIComponent('output-' + outputId)}`);
export const editSceneHref = (sceneId: string) => edit(`scene_id=${encodeURIComponent(sceneId)}`);
export const editSourceHref = (sourceId: string) => edit(`source=${encodeURIComponent(sourceId)}`);
export const editStoryHref = (scriptId: string) => edit(`script_id=${encodeURIComponent(scriptId)}`);
export const editHref = '/?view=edit';
