# API source provenance

This Cloudflare Worker API was moved into this repository from the public Vue
template history so online demo backend code has one owner. The source snapshot
is `npora/admin-template` commit
`f87ee58872ce81d44e13d19a64eb6712ee34737a`, path `workers/api/`.

The Vue and React templates now contain only the frontend, shared API contract,
and MySQL schema/migrations. This copy is the online demo's D1 implementation;
the numbered migrations here are for D1 and must not be applied to MySQL.

When updating the API contract, keep this implementation, the Vue and React
frontend clients, and both templates' `database/mysql/` migrations aligned.
Never copy local `.local`, `.dev.vars`, `.env`, generated seed credentials, or
other secrets into this directory. Demo credentials and platform secrets belong
to their isolated deployment configuration.
