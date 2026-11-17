# AGENTS.md

## Project Rules

1. All user-facing content must support internationalization through the project's existing i18n system. Never hardcode UI strings in components or concatenate translated sentence fragments. This includes labels, buttons, placeholders, tooltips, accessibility text, dialogs, status messages, validation, notifications, and error summaries. Add or update translations for every supported locale with matching keys and interpolation parameters. Use English as the source/fallback language, not as the only display language. Ensure already-open views update when the language changes, and verify at least English and Simplified Chinese. Keep protocol names, addresses, user-provided content, and raw diagnostic details unchanged where appropriate; localize their surrounding explanations.
2. Use React for frontend development.
3. Keep UI styling consistent with native macOS system interfaces.
4. This product targets production users; prioritize clarity, standard macOS layout conventions, and established React interaction patterns over custom visual effects.
