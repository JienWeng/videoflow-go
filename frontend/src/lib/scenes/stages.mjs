/** Disclosure is based on saved content, never permission to run a provider call. */
export function sceneStages({expanded=false,shots=0,storyboard=false,rendered=false,assets=0,all=false}={}) {
 const progress=rendered?4:storyboard?3:shots?2:expanded?1:0;
 return [
  {key:'scene',label:'Story & cast',visible:true,open:progress===0},
  {key:'shots',label:'Shots',visible:all||progress>=1,open:progress===1},
  {key:'assets',label:'Scene assets',visible:all||progress>=2||assets>0,open:false},
  {key:'storyboard',label:'Storyboard',visible:all||progress>=2,open:progress===2},
  {key:'render',label:'Video',visible:all||progress>=3,open:progress>=3}
 ];
}
