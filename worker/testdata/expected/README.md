# Worker OCR Fixture Expectations

This directory stores optional human-readable notes for fixtures under `worker/testdata/images/`.

- Integration tests derive expected truth values from fixture filenames.
- Files here are documentation-only and are not parsed for test assertions.
- Keep fixture naming aligned with:
  - `valid__species-{species_name}__cp-{cp}__hp-{hp}__iv-{atk}-{def}-{sta}__lvl-{level_x10}.{png|jpg|jpeg}`
  - `invalid__pokemon_appraisal_not-stabilized[__{case}].{png|jpg|jpeg}`
  - `invalid__pokemon_with_no_appraisal[__{case}].{png|jpg|jpeg}`
