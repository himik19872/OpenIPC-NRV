import React, { useEffect, useState } from 'react';
import { View, Text, StyleSheet, FlatList, RefreshControl } from 'react-native';
import { camerasApi } from '../api/client';

export default function DashboardScreen() {
  const [cameras, setCameras] = useState<any[]>([]);
  const [refreshing, setRefreshing] = useState(false);

  const load = async () => {
    try {
      const { data } = await camerasApi.list();
      setCameras(data);
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => { load(); }, []);

  const onRefresh = async () => {
    setRefreshing(true);
    await load();
    setRefreshing(false);
  };

  const onlineCount = cameras.filter((c) => c.is_online).length;

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Дашборд</Text>

      <View style={styles.statsRow}>
        <View style={styles.statCard}>
          <Text style={styles.statValue}>{cameras.length}</Text>
          <Text style={styles.statLabel}>Камер</Text>
        </View>
        <View style={styles.statCard}>
          <Text style={[styles.statValue, { color: '#52c41a' }]}>{onlineCount}</Text>
          <Text style={styles.statLabel}>Онлайн</Text>
        </View>
        <View style={styles.statCard}>
          <Text style={[styles.statValue, { color: '#ff4d4f' }]}>{cameras.length - onlineCount}</Text>
          <Text style={styles.statLabel}>Офлайн</Text>
        </View>
      </View>

      <FlatList
        data={cameras}
        keyExtractor={(item) => item.id}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}
        renderItem={({ item }) => (
          <View style={styles.cameraItem}>
            <View style={{ flex: 1 }}>
              <Text style={styles.cameraName}>{item.name}</Text>
              <Text style={styles.cameraLocation}>{item.location || '-'}</Text>
            </View>
            <View style={[styles.statusDot, { backgroundColor: item.is_online ? '#52c41a' : '#ff4d4f' }]} />
          </View>
        )}
        ListEmptyComponent={<Text style={{ textAlign: 'center', color: '#888', marginTop: 40 }}>Нет камер</Text>}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: '#f5f5f5', padding: 16 },
  title: { fontSize: 24, fontWeight: '700', marginBottom: 16 },
  statsRow: { flexDirection: 'row', gap: 12, marginBottom: 20 },
  statCard: {
    flex: 1, backgroundColor: '#fff', borderRadius: 12, padding: 16,
    alignItems: 'center', elevation: 2, shadowColor: '#000', shadowOpacity: 0.05, shadowRadius: 4,
  },
  statValue: { fontSize: 28, fontWeight: '700', color: '#1677ff' },
  statLabel: { fontSize: 12, color: '#888', marginTop: 4 },
  cameraItem: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: '#fff',
    borderRadius: 10, padding: 14, marginBottom: 8,
    elevation: 1, shadowColor: '#000', shadowOpacity: 0.04, shadowRadius: 3,
  },
  cameraName: { fontSize: 16, fontWeight: '600' },
  cameraLocation: { fontSize: 12, color: '#888', marginTop: 2 },
  statusDot: { width: 12, height: 12, borderRadius: 6 },
});