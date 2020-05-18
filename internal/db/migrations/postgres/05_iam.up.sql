BEGIN;

CREATE OR REPLACE FUNCTION update_time_column() RETURNS TRIGGER 
SET SCHEMA
  'public' LANGUAGE plpgsql AS $$
BEGIN
   IF row(NEW.*) IS DISTINCT FROM row(OLD.*) THEN
      NEW.update_time = now(); 
      RETURN NEW;
   ELSE
      RETURN OLD;
   END IF;
END;
$$;


CREATE TABLE iam_group (
    public_id wt_public_id not null primary key,
    create_time wt_timestamp,
    update_time wt_timestamp,
    name text,
    description text,
    scope_id wt_public_id NOT NULL REFERENCES iam_scope(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    unique(name, scope_id),
    disabled BOOLEAN NOT NULL default FALSE
  );
  
CREATE TRIGGER update_iam_group_update_time 
BEFORE UPDATE ON iam_group FOR EACH ROW EXECUTE PROCEDURE update_time_column();

CREATE TABLE iam_group_member_user (
    create_time wt_timestamp,
    group_id wt_public_id NOT NULL REFERENCES iam_group(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    member_id wt_public_id NOT NULL REFERENCES iam_user(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    primary key (group_id, member_id)
  );


CREATE VIEW iam_group_member AS
SELECT
  *, 'user' as type
FROM iam_group_member_user;

CREATE TABLE iam_group_member_type_enm (
    string text NOT NULL primary key CHECK(string IN ('unknown', 'user'))
  );
INSERT INTO iam_group_member_type_enm (string)
values
  ('unknown'),
  ('user');



CREATE TABLE iam_auth_method (
    public_id wt_public_id not null primary key, 
    create_time wt_timestamp,
    update_time wt_timestamp,
    name text,
    description text,
    scope_id wt_public_id NOT NULL REFERENCES iam_scope_organization(scope_id) ON DELETE CASCADE ON UPDATE CASCADE,
    unique(name, scope_id),
    disabled BOOLEAN NOT NULL default FALSE,
    type text NOT NULL
  );


CREATE TABLE iam_role (
    public_id wt_public_id not null primary key,
    create_time wt_timestamp,
    update_time wt_timestamp,
    name text,
    description text,
    scope_id wt_public_id NOT NULL REFERENCES iam_scope(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    unique(name, scope_id),
    disabled BOOLEAN NOT NULL default FALSE
  );



CREATE TABLE iam_auth_method_type_enm (
    string text NOT NULL primary key CHECK(string IN ('unknown', 'userpass', 'oidc'))
  );
INSERT INTO iam_auth_method_type_enm (string)
values
  ('unknown'),
  ('userpass'),
  ('oidc');
ALTER TABLE iam_auth_method
ADD
  FOREIGN KEY (type) REFERENCES iam_auth_method_type_enm(string);

CREATE TABLE iam_action_enm (
    string text NOT NULL primary key CHECK(
      string IN (
        'unknown',
        'list',
        'create',
        'update',
        'edit',
        'delete',
        'authen'
      )
    )
  );

INSERT INTO iam_action_enm (string)
values
  ('unknown'),
  ('list'),
  ('create'),
  ('update'),
  ('edit'),
  ('delete'),
  ('authen');

CREATE TABLE iam_role_user (
    create_time wt_timestamp,
    role_id wt_public_id NOT NULL REFERENCES iam_role(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    principal_id wt_public_id NOT NULL REFERENCES iam_user(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    primary key (role_id, principal_id)
  );

CREATE TABLE iam_role_group (
    create_time wt_timestamp,
    role_id wt_public_id NOT NULL REFERENCES iam_role(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    principal_id wt_public_id NOT NULL REFERENCES iam_group(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    primary key (role_id, principal_id)
  );

CREATE VIEW iam_assigned_role AS
SELECT
  -- intentionally using * to specify the view which requires that the concrete role assignment tables match
  *, 'user' as type
FROM iam_role_user
UNION
select
  -- intentionally using * to specify the view which requires that the concrete role assignment tables match
  *, 'group' as type
from iam_role_group;



CREATE TABLE iam_role_grant (
    public_id wt_public_id not null primary key,
    create_time wt_timestamp,
    update_time wt_timestamp,
    description text,
    role_id wt_public_id NOT NULL REFERENCES iam_role(public_id) ON DELETE CASCADE ON UPDATE CASCADE,
    "grant" text NOT NULL
  );

  COMMIT;