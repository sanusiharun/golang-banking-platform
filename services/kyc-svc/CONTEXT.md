# kyc-svc

Customer identity verification. Owns the `Verification` concept; KTP OCR extraction is its first verification type. Designed to be extractable into a standalone OCR/KYC provider product later, so it does not depend on this platform's `auth-svc` or `audit-svc`.

## Language

**Verification** / **VerificationRequest**:
A request to verify some aspect of a customer's identity, distinguished by `type` (e.g. `ktp_ocr`). The general concept this service owns; new verification types are added without renaming or re-splitting the service.
_Avoid_: OCR request, scan

**KTP OCR**:
The `ktp_ocr` verification type — extracting structured fields from a photographed Indonesian KTP (ID card) and scoring the result. The only verification type built in this round.

**OCR Confidence**:
Per-field confidence value reported by the OCR engine itself, reflecting how sure the engine is about what it read.
_Avoid_: accuracy (accuracy requires ground truth and only applies to offline benchmarking, not live requests)

**Field Validity Score**:
A score derived from business rules independent of the OCR engine — e.g. NIK is 16 digits with a valid province/regency code, birth date parses, required fields are non-empty. Measures whether the *extracted value* looks like a real KTP field, not how sure the OCR was.

**Image Quality Score**:
A score computed before OCR runs, measuring how trustworthy the input image is (blur, glare, crop, resolution). Explains *why* OCR confidence or field validity might be low.

**Accuracy**:
A ground-truth-based metric, only meaningful in the offline benchmark harness (comparing engine output against a labeled KTP dataset). Never used to describe a live request's result.
_Avoid_: confidence, score (these are runtime signals with no ground truth)

**Benchmark**:
An offline, non-production harness that runs candidate OCR engines against a labeled KTP dataset to measure accuracy and decide which engine `kyc-svc` uses in production. Not a running service.

## Outcome States

- **processing** — transient; the verification is in flight (synchronous request, relevant mainly to the audit trail)
- **verified** — OCR succeeded, all expected fields extracted, field validity checks passed, image quality above threshold
- **needs_review** — OCR ran, but at least one score is below threshold (low OCR confidence, poor image quality, or a field-validity failure like a malformed NIK). Result is still returned with all three scores, but flagged as not auto-trustworthy.
- **rejected** — OCR could not extract usable data, or the input isn't a readable KTP at all

## Fields (v1 scope)

NIK, Nama, Tempat/Tgl Lahir, Jenis Kelamin, Alamat (RT/RW, Kel/Desa, Kecamatan), Agama, Status Perkawinan, Kewarganegaraan, Berlaku Hingga.
_Excluded from v1_: Gol. Darah, Pekerjaan — low-signal for banking KYC, added later without a model change if ever needed.
