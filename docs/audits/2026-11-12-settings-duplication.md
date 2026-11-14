# Settings connection duplication

The supplied screenshot showed the same OpenRouter status twice: once in the provider disclosure summary and again as a badge inside it. An optional “Add another OpenRouter connection” form also preceded the primary provider list, making the setup appear duplicated.

The provider summary now carries the single configured/unconfigured status. Saved-key source information remains available inside configured providers. The optional named-connection form follows the provider list under “Additional connections,” explaining that it is only for a separate account or configuration. Provider records, credentials, routes and save/test behavior are unchanged.

Verification: Svelte check found zero errors/warnings and the production build passed. The 14 desktop/mobile application checks passed with zero page errors. Added assertions verify one unconfigured status for OpenRouter, the additional form appearing after the primary provider, and its explanatory copy. Existing assertions verify credential Save enablement and dirty-navigation protection. Real credential writes and provider calls were not performed. The updated running page was visually inspected after reloading its previous frontend bundle.
