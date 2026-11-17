/**
 * Keep a floating assistant's header and edges within the workspace.
 * @param {{x:number,y:number}} position
 * @param {{width:number,height:number}} bounds
 * @param {number} width
 * @param {number} height
 */
export function clampWindow(position,bounds,width,height) {
 const padX=Math.min(12,Math.max(0,(bounds.width-width)/2));
 const padY=Math.min(12,Math.max(0,(bounds.height-height)/2));
 const maxX=Math.max(padX,bounds.width-width-padX),maxY=Math.max(padY,bounds.height-height-padY);
 return {x:Math.round(Math.max(padX,Math.min(Number.isFinite(position.x)?position.x:maxX,maxX))),y:Math.round(Math.max(padY,Math.min(Number.isFinite(position.y)?position.y:maxY,maxY)))};
}
/**
 * Read only known local layout fields. Malformed storage must not hide the window.
 * @param {string|null} raw
 * @returns {{mode:'docked'|'floating',position:{x:number,y:number}|null,width?:number,height?:number}|null}
 */
export function readWindowLayout(raw) {
 try {
  const value=JSON.parse(raw??'null');
  if(!value||!['docked','floating'].includes(value.mode))return null;
  return {mode:value.mode,position:Number.isFinite(value.x)&&Number.isFinite(value.y)?{x:value.x,y:value.y}:null,width:Number.isFinite(value.width)&&value.width>=240&&value.width<=760?value.width:undefined,height:Number.isFinite(value.height)&&value.height>=360&&value.height<=10000?value.height:undefined};
 } catch {return null;}
}

/**
 * Bound a preferred height to the available workspace, optionally keeping its top fixed.
 * @param {number} workspaceHeight
 * @param {number} [top]
 */
export function windowHeightLimits(workspaceHeight, top = 12) {
 const max=Math.max(0,Math.floor(workspaceHeight-Math.max(12,top)-12));
 return {min:Math.min(360,max),max};
}
/** @param {number} height @param {number} workspaceHeight @param {number} [top] */
export function clampWindowHeight(height, workspaceHeight, top = 12) {
 const {min,max}=windowHeightLimits(workspaceHeight,top);
 return Math.round(Math.max(min,Math.min(Number.isFinite(height)?height:680,max)));
}
