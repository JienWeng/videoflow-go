export const creationSteps = [
  {key:'story',label:'Shape the story',detail:'Develop the idea and identify the characters.'},
  {key:'scenes',label:'Build the scenes',detail:'Turn the story into scene-by-scene action.'},
  {key:'shots',label:'Plan the shots',detail:'Set the framing and visual direction.'},
  {key:'visuals',label:'Create storyboard images',detail:'Generate the images you will review before video.'},
  {key:'review',label:'Review references',detail:'Check character appearance and approve each scene’s images.'},
  {key:'render',label:'Render scene clips',detail:'Start each clip after approval. Provider charges may apply.'}
];
export function stepsForCreation(characterReferences=false){
 return characterReferences?[...creationSteps.slice(0,3),
 {key:'characters',label:'Prepare character references',detail:'Generate sheets for characters without images; reuse existing references.'},
 {key:'character_review',label:'Approve character designs',detail:'Review each character sheet before it is used in scene images.'},
 ...creationSteps.slice(3)]:creationSteps;
}
/** @param {string} status @param {string[]} stages @param {boolean} characterReferences */
export function creationProgress(status,stages=[],characterReferences=false){
 const steps=stepsForCreation(characterReferences);
 const completed=new Set(stages.filter(key=>key!=='render'));
 const rendering=status==='rendering'||stages.includes('render');
 if(status==='completed'&&rendering)completed.add('render');
 const stopped=['failed','reconciliation_required'].includes(status);
 const characterReview=status==='awaiting_character_review';
 const review=status==='awaiting_reference_review'||characterReview;
 const next=characterReview?'character_review':review?'review':rendering?'render':steps.find(step=>!completed.has(step.key))?.key??'review';
 return {completed,stopped,review,characterReview,current:next,rendering,title:stopped?'Your story needs attention':characterReview?'Your character references are ready':review?'Your storyboard is ready':status==='completed'?(rendering?'Your video is ready':'Your story is ready'):rendering?'Rendering your scene clips':'Bringing your story to life'};
}
