# Collapsible sidebar

Desktop navigation now toggles between the existing 224px sidebar and a 64px icon rail using the top-left control. The browser remembers the preference. Links retain accessible names, hover titles and active-route indicators. More tools remains reachable through a menu, and Search/Activity use compact controls. The current page stays mounted, preserving chat drafts. Reduced-motion preferences disable the width transition.

Mobile retains the full-label navigation drawer, independently of the desktop collapse preference. Preference storage failures leave the toggle functional for the current visit.

Verification: seven focused browser checks passed with zero page errors, covering workspace width, retained drafts, named links, tool/search/activity controls, keyboard expansion, reload/routing persistence, mobile navigation and viewport transitions. The 14 application regression checks also passed. Svelte check found zero errors/warnings; production build passed. The running interface was visually inspected. No backend/provider/credential changes were made.
