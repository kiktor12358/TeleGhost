# CI/CD Fix Report (v1.0.3 - Third Attempt)

## Issue
`Compile i2pd (Android)` failed with "Could not find Boost libs".
The `BOOST_ROOT` variable pointed to `.../boost_prebuilt` (the clone root), but the actual libraries are in `.../boost_prebuilt/boost-1_78_0/...`.
My previous `android.yml` logic likely fell back to the root check or `find` returned the root directory itself.

## Fix Applied
1. Updated `.github/workflows/android.yml`:
   - Added `ls -F boost_prebuilt/` to debug structure.
   - Refined `BOOST_SUBDIR` detection: `find ... | grep -v "^boost_prebuilt$"` to avoid matching the parent dir.
   - Prioritized finding the subdirectory over assuming root content.
2. Pushed commit `62f6b02`.

## Expected Outcome
- `BOOST_ROOT` will be set to `.../boost_prebuilt/boost-1_78_0`.
- Build script will find libs at `$BOOST_ROOT/$ARCH/lib`.
- `cmake` will succeed.
