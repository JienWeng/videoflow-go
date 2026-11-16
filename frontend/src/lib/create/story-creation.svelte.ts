const state=$state({open:false});
export const storyCreation={get open(){return state.open},set open(value:boolean){state.open=value},show(){if(typeof window!=='undefined'&&window.location.pathname==='/create'){document.getElementById('idea')?.focus();return;}state.open=true}};
