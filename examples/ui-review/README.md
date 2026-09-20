# UI review fixture

A self-contained static page for validating Veda's screenshot comparison. It has no external assets, scripts, services, or dependencies. The action button is decorative.

Example investigation objective: "In examples/ui-review/index.html, change only the .action button background from #899f91 to #165d46 to increase contrast. Preserve all text, layout, and other styles. Suggest one independent change."

Enable suggested changes and UI previews, and set Preview page to `examples/ui-review/index.html`. Veda should save an applicable HTML patch plus actual before, after, and pixel-difference PNGs. A successful capture is evidence of that page's appearance, not application behavior or accessibility compliance.
