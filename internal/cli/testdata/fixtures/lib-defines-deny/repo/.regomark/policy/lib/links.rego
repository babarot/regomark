package lib.links

broken(l) if l.kind in {"missing", "outside"}

deny contains "a library is not a policy" if true
