/** Chat requests are recorded as intent_agent by the local text engine.
 * Internal planning/image turns are not conversation messages.
 * @param {any[]} turns
 */
export function conversationTurns(turns){return turns.filter(t=>['chat','intent_agent','specialist_review'].includes(t.kind)&&typeof t.request_json?.user_message==='string'&&t.request_json.user_message.trim())}
/** Preserve the richer current-tab view only when it matches the saved conversation.
 * New server messages always win over a stale panel snapshot.
 * @param {any[]} server @param {any} cached @returns {any[]}
 */
export function conversationView(server,cached){
 if(!Array.isArray(cached)||!cached.length||!cached.every(m=>m&&['user','assistant'].includes(m.role)&&typeof m.text==='string'))return server;
 /** @param {any[]} messages */
 const users=messages=>messages.filter(m=>m.role==='user').map(m=>m.text);
 if(JSON.stringify(users(server))!==JSON.stringify(users(cached)))return server;
 const lastUser=server.map(m=>m.role).lastIndexOf('user');
 const replies=server.slice(lastUser+1).filter(m=>m.role==='assistant');
 if(!replies.every(reply=>cached.some(m=>!m.op&&m.role==='assistant'&&m.text===reply.text)))return server;
 return cached.map(m=>m.op?.status==='running'?{...m,op:undefined,text:m.text||`${m.op.label} was running when you left. Check Activity for its current status before retrying.`,activity:[...(m.activity??[]),{kind:'tool',title:m.op.label,status:'pending',detail:'Check Activity for the current status. This saved view does not restart the request.'}]}:m);
}
