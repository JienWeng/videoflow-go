// The names the owner knows each video service by, keyed by the app's service id.
const names = /** @type {Record<string, string>} */ ({ fal: 'fal.ai', byteplus: 'BytePlus', openrouter: 'OpenRouter', argolink: 'ArgoLink', loopling: 'Loopling', replicate: 'Replicate', kie: 'Kie.ai', wavespeed: 'WaveSpeed', runway: 'Runway' });

/** @param {string | null | undefined} provider */
export function serviceName(provider) {
 return (provider && names[provider]) || 'The video service';
}
