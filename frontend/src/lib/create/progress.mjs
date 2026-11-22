export const creationSteps = [
  {key:'story',label:'Shape the story',detail:'Develop the idea and identify the characters.'},
  {key:'scenes',label:'Build the scenes',detail:'Turn the story into scene-by-scene action.'},
  {key:'shots',label:'Plan the shots',detail:'Set the framing and visual direction.'},
  {key:'visuals',label:'Prepare scene pictures',detail:'Make a storyboard picture for each scene whose clips start from one. Scenes whose clips use their connected pictures need none.'},
  {key:'render',label:'Render scene clips',detail:'Start each clip when ready. Provider charges may apply.'}
];
export function stepsForCreation(characterReferences=false,automaticAssets=false){
 return characterReferences?[...creationSteps.slice(0,3),
 ...(automaticAssets?[{key:"assets",label:"Prepare props and locations",detail:"Check the library, reuse suitable assets and generate missing references."}]:[]),
 {key:'characters',label:'Prepare character references',detail:'Generate sheets for characters without images; reuse existing references.'},
 ...creationSteps.slice(3)]:creationSteps;
}
/** storyboardsMade is how many storyboard pictures the story made: 0 when every scene's clips use connected pictures instead, null while unknown.
 * @param {string} status @param {string[]} stages @param {boolean} characterReferences @param {boolean} automaticAssets @param {number|null} storyboardsMade */
export function creationProgress(status,stages=[],characterReferences=false,automaticAssets=false,storyboardsMade=null){
 const noStoryboards=storyboardsMade===0;
 const steps=stepsForCreation(characterReferences,automaticAssets);
 const completed=new Set(stages.filter(key=>key!=='render'));
 const rendering=status==='rendering'||stages.includes('render');
 if(status==='completed'&&rendering)completed.add('render');
 const stopped=['failed','reconciliation_required'].includes(status);
 const characterReview=status==='awaiting_character_review';
 const review=['awaiting_reference_review','ready_to_render'].includes(status)||characterReview;
 const next=characterReview?'visuals':review?'render':rendering?'render':steps.find(step=>!completed.has(step.key))?.key??'review';
 return {noStoryboards,completed,stopped,review,characterReview,current:next,rendering,title:stopped?'Your story needs attention':characterReview?(automaticAssets?'Your story references are ready':'Your character references are ready'):review?(noStoryboards?'Your story is ready for video':'Your storyboard is ready'):status==='completed'?(rendering?'Your video is ready':'Your story is ready'):rendering?'Rendering your scene clips':'Bringing your story to life'};
}
