package lib.links

broken(l) if l.kind in {"missing", "outside"}

# A library must not define deny, default included
default deny := {"a library is not a policy"}
