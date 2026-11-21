const state=$state({open:false});
export const storyCreation={get open(){return state.open},set open(value:boolean){state.open=value},show(){state.open=true}};
