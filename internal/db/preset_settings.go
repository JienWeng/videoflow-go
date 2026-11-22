package db

// CompareAppSetting atomically replaces an observed value; concurrent editors must reload.
func (d *DB) CompareAppSetting(key, old, value string) (bool, error) {
	if old == "" {
		r, e := d.conn.Exec("INSERT OR IGNORE INTO app_settings(key,value) VALUES(?,?)", key, value)
		if e != nil {
			return false, e
		}
		n, e := r.RowsAffected()
		return n == 1, e
	}
	r, e := d.conn.Exec("UPDATE app_settings SET value=? WHERE key=? AND value=?", value, key, old)
	if e != nil {
		return false, e
	}
	n, e := r.RowsAffected()
	return n == 1, e
}
