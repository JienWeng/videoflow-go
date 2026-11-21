import {redirect} from '@sveltejs/kit';
import type {PageLoad} from './$types';
// Legacy route: keep old links and bookmarks working, but land on the integrated page.
export const load: PageLoad = ({url}) => {
 const q = new URLSearchParams(url.search);
 q.set('view','story');q.set('new_story','1');
 throw redirect(307, '/' + '?' + q.toString());
};
