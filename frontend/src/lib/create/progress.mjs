export const creationSteps = [
  {key:'story',label:'Shape the story',detail:'Develop the idea and identify the characters.'},
  {key:'scenes',label:'Build the scenes',detail:'Turn the story into scene-by-scene action.'},
  {key:'shots',label:'Plan the shots',detail:'Set the framing and visual direction.'},
  {key:'visuals',label:'Create storyboard images',detail:'Generate the images you will review before video.'},
  {key:'review',label:'Review references',detail:'Check character appearance and approve each scene’s images.'},
  {key:'render',label:'Render scene clips',detail:'Start each clip after approval. Provider charges may apply.'}
];
/** @param {string} status @param {string[]} stages */
export function creationProgress(status,stages=[]){
 const completed=new Set(stages.filter(key=>key!=='render'));
 const rendering=status==='rendering'||stages.includes('render');
 if(status==='completed'&&rendering)completed.add('render');
 const stopped=['failed','reconciliation_required'].includes(status);
 const review=status==='awaiting_reference_review';
 const next=review?'review':rendering?'render':creationSteps.find(step=>!completed.has(step.key))?.key??'review';
 return {completed,stopped,review,current:next,rendering,title:stopped?'Your story needs attention':review?'Your storyboard is ready':status==='completed'?(rendering?'Your video is ready':'Your story is ready'):rendering?'Rendering your scene clips':'Bringing your story to life'};
}
