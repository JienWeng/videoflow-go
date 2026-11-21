/** Whole-scene versions are alternatives; every unique unit of a grouped plan must finish.
 * @param {Array<{status?:string,scene_id?:string|null,shot_id?:string|null,request_json?:any,render_unit?:any,render_plan_id?:string}>} jobs
 * @returns {Set<string>}
 */
export function completeSceneRenders(jobs){
 const complete=new Set(),plans=new Map();
 for(const job of jobs){
  if(job.status!=='succeeded'||!job.scene_id||job.shot_id)continue;
  let request=job.request_json??{};if(typeof request==='string'){try{request=JSON.parse(request)}catch{continue}}
  const unit=job.render_unit??request.render_unit;if(!unit?.key){complete.add(job.scene_id);continue}
  const plan=job.render_plan_id??request.render_plan_id??unit.key.split(':')[0];
  if(!plan||!Number.isInteger(unit.count)||unit.count<1||unit.count>600||!Number.isInteger(unit.index)||unit.index<0||unit.index>=unit.count)continue;
  const key=job.scene_id+':'+plan;let group=plans.get(key);if(!group){group={scene:job.scene_id,count:unit.count,seen:new Set()};plans.set(key,group)}if(group.count===unit.count)group.seen.add(unit.index);
 }
 for(const group of plans.values())if(group.seen.size===group.count)complete.add(group.scene);return complete;
}
