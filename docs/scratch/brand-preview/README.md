# Archived brand preview experiment

`preview_temp_test.go.txt` preserves the original preview experiment unchanged.
It references the future `Pose`, `Sample`, `WidgetProfile` and `Render` APIs from
the installer/brand/motion plan. That module has not been implemented, and this
file only wrote preview images; it contained no regression assertions.

It previously lived alone under `internal/brand`, where Go discovered it as a
package test and failed the ordinary application checks. Keep this experiment
outside compiled packages until the brand work is implemented. This archive
does not implement or approve that separate plan.
