-- PRATHAM HOSPITAL DIRECTORY - Relational Schema
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Hospitals Table
CREATE TABLE IF NOT EXISTS hospitals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_sno INTEGER,
    hospital_name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    address TEXT,
    location VARCHAR(255),
    district VARCHAR(100),
    state VARCHAR(100),
    type VARCHAR(50) NOT NULL DEFAULT 'Pratham', -- 'Pratham', 'Pratham Neo', 'Network', 'Blocked'
    public_visibility BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hospitals_norm_name ON hospitals(normalized_name);
CREATE INDEX IF NOT EXISTS idx_hospitals_location ON hospitals(location);
CREATE INDEX IF NOT EXISTS idx_hospitals_district ON hospitals(district);
CREATE INDEX IF NOT EXISTS idx_hospitals_state ON hospitals(state);
CREATE INDEX IF NOT EXISTS idx_hospitals_type ON hospitals(type);
CREATE INDEX IF NOT EXISTS idx_hospitals_public_vis ON hospitals(public_visibility);

-- Specializations Table
CREATE TABLE IF NOT EXISTS specializations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical_name VARCHAR(255) NOT NULL UNIQUE,
    normalized_name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_specializations_norm_name ON specializations(normalized_name);

-- Hospital Specializations (Association)
CREATE TABLE IF NOT EXISTS hospital_specializations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
    specialization_id UUID NOT NULL REFERENCES specializations(id) ON DELETE CASCADE,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_hospital_specialization UNIQUE(hospital_id, specialization_id)
);

CREATE INDEX IF NOT EXISTS idx_hosp_spec_hosp ON hospital_specializations(hospital_id);
CREATE INDEX IF NOT EXISTS idx_hosp_spec_spec ON hospital_specializations(specialization_id);

-- Doctors Table
CREATE TABLE IF NOT EXISTS doctors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    qualifications VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doctors_norm_name ON doctors(normalized_name);

-- Hospital Doctors (Hospital Affiliation)
CREATE TABLE IF NOT EXISTS hospital_doctors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
    doctor_id UUID NOT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    display_name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    needs_specialty_review BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_hospital_doctor UNIQUE(hospital_id, doctor_id)
);

CREATE INDEX IF NOT EXISTS idx_hosp_doc_hosp ON hospital_doctors(hospital_id);
CREATE INDEX IF NOT EXISTS idx_hosp_doc_doc ON hospital_doctors(doctor_id);
CREATE INDEX IF NOT EXISTS idx_hosp_doc_review ON hospital_doctors(needs_specialty_review);

-- Doctor Specializations (supports many-to-many doctor-specialty links at a hospital)
CREATE TABLE IF NOT EXISTS doctor_specializations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_doctor_id UUID NOT NULL REFERENCES hospital_doctors(id) ON DELETE CASCADE,
    hospital_specialization_id UUID REFERENCES hospital_specializations(id) ON DELETE SET NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_doctor_specialization UNIQUE(hospital_doctor_id, hospital_specialization_id)
);

CREATE INDEX IF NOT EXISTS idx_doc_spec_hdoc ON doctor_specializations(hospital_doctor_id);
CREATE INDEX IF NOT EXISTS idx_doc_spec_hspec ON doctor_specializations(hospital_specialization_id);

-- Custom Field Definitions
CREATE TABLE IF NOT EXISTS custom_field_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    field_key VARCHAR(100) NOT NULL UNIQUE,
    label VARCHAR(255) NOT NULL,
    entity_scope VARCHAR(50) NOT NULL, -- 'hospital', 'specialization', 'doctor'
    data_type VARCHAR(50) NOT NULL, -- 'short_text', 'long_text', 'number', 'date', 'single_select', 'multi_select', 'boolean'
    required BOOLEAN NOT NULL DEFAULT false,
    searchable BOOLEAN NOT NULL DEFAULT false,
    filterable BOOLEAN NOT NULL DEFAULT false,
    options JSONB,
    display_order INTEGER NOT NULL DEFAULT 0,
    is_public BOOLEAN NOT NULL DEFAULT true,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Custom Field Values
CREATE TABLE IF NOT EXISTS custom_field_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    definition_id UUID NOT NULL REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
    entity_id UUID NOT NULL,
    value TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_custom_val_def_entity UNIQUE(definition_id, entity_id)
);

CREATE INDEX IF NOT EXISTS idx_custom_vals_entity ON custom_field_values(entity_id);
CREATE INDEX IF NOT EXISTS idx_custom_vals_def ON custom_field_values(definition_id);

-- Import Batches Audit
CREATE TABLE IF NOT EXISTS import_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_name VARCHAR(255) NOT NULL,
    upload_type VARCHAR(50) NOT NULL, -- 'hospital_master', 'specialization_doctor'
    uploader VARCHAR(100) NOT NULL DEFAULT 'admin',
    total_rows INTEGER NOT NULL DEFAULT 0,
    accepted_rows INTEGER NOT NULL DEFAULT 0,
    created_rows INTEGER NOT NULL DEFAULT 0,
    updated_rows INTEGER NOT NULL DEFAULT 0,
    skipped_rows INTEGER NOT NULL DEFAULT 0,
    invalid_rows INTEGER NOT NULL DEFAULT 0,
    review_rows INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT 'completed',
    summary_notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Import Row Logs
CREATE TABLE IF NOT EXISTS import_row_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    row_number INTEGER NOT NULL,
    source_data JSONB NOT NULL,
    status VARCHAR(50) NOT NULL, -- 'created', 'updated', 'skipped', 'invalid', 'needs_review'
    error_message TEXT,
    recommended_action TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_import_row_batch ON import_row_logs(batch_id);

-- Admin Users
CREATE TABLE IF NOT EXISTS admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'admin',
    active BOOLEAN NOT NULL DEFAULT true,
    last_login TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- System Settings
CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT NOT NULL,
    description TEXT,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Insert Default Settings
INSERT INTO system_settings (key, value, description)
VALUES 
('show_blocked_in_public', 'false', 'Whether blocked hospitals appear in public directory search'),
('site_title', 'Pratham Hospital Directory', 'Public Directory title'),
('contact_email', 'admin@pratham-hospitals.marketrates.in', 'Support and contact email')
ON CONFLICT (key) DO NOTHING;

